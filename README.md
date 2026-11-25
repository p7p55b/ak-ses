# AK-SES

Outil léger d'administration et de contrôle à distance (Remote Desktop) en Go avec interface Web.

## Avertissement Légal / Disclaimer

Ce projet est strictement destiné à un usage légitime : assistance technique autorisée, administration de ses propres machines et tests en environnement contrôlé (lab).

- **Tout usage comme RAT, logiciel espion ou prise de contrôle sans consentement écrit préalable est illégal** (art. 323-1 du Code pénal en France, CFAA aux USA).
- Les auteurs déclinent toute responsabilité quant à l'usage fait de ce logiciel.

---

## Déploiement

Des binaires précompilés prêts à l'emploi sont disponibles dans les **Releases** (Linux & Windows).

### 1. Serveur

#### Lancement direct
```bash
./server-linux -port 8080 -bin-dir ./bin
```

#### En service systemd (Linux)
```ini
# /etc/systemd/system/ak-ses.service
[Unit]
Description=ak-ses Remote Server
After=network.target

[Service]
Type=simple
WorkingDirectory=/root
Environment="HOME=/root" "GOPATH=/root/go"
ExecStart=/root/server-linux -port 8080 -bin-dir /root/bin
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now ak-ses
```

Le panel d'administration est accessible sur `http://<IP_OU_DOMAINE>:8080`.

---

### 2. Agent (Client Windows)

Deux manières de déployer l'agent sur la machine cible :

#### Option A : Compilation sur-mesure depuis le panel admin
1. Rendez-vous sur le panel (`http://<IP_OU_DOMAINE>:8080`).
2. Dans la section **BUILD_AGENT**, entrez votre domaine/IP et cliquez sur **`[ ⚙ COMPILER AGENT ]`**.
3. Récupérez le binaire avec **`[ ⇩ DL EXE ]`** ou copiez la commande one-liner affichée.

---

## Arguments CLI

### Serveur
- `-port` : Port d'écoute HTTP/WS (défaut: `8080`)
- `-bin-dir` : Dossier contenant les binaires de l'agent (défaut: `./bin`)
- `-agent-src` : Dossier des sources pour la compilation dynamique (défaut: `./agent`)

### Agent
- `-server` : Hôte du serveur relais (défaut: `localhost:8080`)
- `-fps` : Taux de rafraîchissement vidéo (défaut: `20`)
- `-quality` : Qualité JPEG 1-100 (défaut: `50`)
- `-id` : Identifiant personnalisé de la machine

---