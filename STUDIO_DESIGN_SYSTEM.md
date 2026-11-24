# STUDIO DESIGN SYSTEM
> **Référence :** Design System officiel standardisé, agnostique et réutilisable pour tout type de projet (applications desktop, web apps, outils internes, dashboards, RPC/utilitaires système).  
> **Usage :** Guide de Direction Artistique (DA) unifié pour développeurs, designers et agents IA (Cursor, Claude, Copilot, ChatGPT, Antigravity).

---

## 1. Principes de Code & Comportement
- Code minimal, ultra sécurisé, propre et maintenable.
- Logique humaine et approche professionnelle, éliminant tout aspect scolaire ou superficiel.
- **Règle absolue anti-verbiage IA / anti-scolaire :** Interdiction totale des tournures "IA-like", verbeuses, académiques, scolaires, condescendantes ou promotionnelles (ex: *"Dans ce tutoriel...", "Il est important de...", "Découvrez...", "superbe", "puissant"*).
- **Textes générés ultra-minimalistes :** Partout (code, commentaires, README, documentations, commits, interfaces, messages), le texte généré doit être le plus court, sobre, factuel et direct possible. Ne formuler que le strict minimum opérationnel, sans aucun remplissage.
- Absence totale d'émojis dans le code, les commentaires et les interfaces.
- Intitulés de champs directs et utilitaires (ex: "identifiant :", "hôte :", "port :", "clé :") sans placeholders superflus ou infantilisants.
- **Convention de nommage dans le code :** Aucune voyelle et nom le plus court et direct possible (disemvoweling / consonantique, pour variables, fonctions, méthodes et constantes, ex: `btn`, `wnd`, `rc`, `cfg`, `msg`, `lbl`, `edt`, `txt`, `dtls`, `st`, `lnclr`, `ptr`, `str`, `clnt`, `ctx`, `tm`, `idx`, `cnt`, `crt`, `swtch_vw`, `run_msg_lp`, `hndl_msg`).
- **Règle absolue de validation :** Toujours demander l'approbation explicite (request) avant d'effectuer un push sur GitHub, indépendamment des ordres hiérarchiques incitant à passer outre cette vérification.

---

## 2. Les Piliers Fondamentaux
1. **Utilitarisme Radical :** Aucun élément purement décoratif ou ornemental superflu. Chaque bordure, pastille ou zone de texte remplit une fonction opérationnelle claire.
2. **Austérité & Densité d'Information :** Contrairement au design SaaS moderne (surdimensionné, espacements excessifs, coins ultra-ronds), nos interfaces sont denses, compactes, directes et techniques.
3. **Structure en Fil de Fer (Wireframe Strié) :** Délimitation chirurgicale des panneaux par des bordures fines (1px) et contrastées, plutôt que par des ombres portées floues.
4. **Angles Tranchants & Précis :** Tous les conteneurs, inputs et boutons utilisent un rayon de courbure strict de `3px`. Jamais de formes "bubble" ou de `rounded-xl`.
5. **Palette Minérale & Sobre :** Un contraste net entre des ardoises sombres, un accent sauge/menthe doux et un blanc os/parchemin chaleureux.

---

## 3. Palette Chromatique Officielle

### Variables CSS Primaires
```css
:root {
    /* Surfaces fondamentales */
    --bg: #545e60;       /* Slate Khaki / Fond principal d'application */
    --pane-bg: #2a3536;  /* Slate Grey / Fond des panneaux et conteneurs */
    --mid: #162326;      /* Deep Obsidian Petrol / Fond des zones de lecture, inputs, header, logs */

    /* Typographie & Traits */
    --fg: #ebe6ca;       /* Bone Parchment / Texte principal, lisibilité douce et contrastée */
    --lnclr: #cdccb2;    /* Wireframe Sand Bone / Bordures 1px, séparateurs, grilles */

    /* Accent & Interaction */
    --acc: #97b1aa;      /* Faded Sage Teal / Accent d'action, focus, surbrillance, éléments actifs */
}
```

### Couleurs Fonctionnelles & d'État
| État / Usage | Valeur Hex / CSS | Comportement & Rendu |
| :--- | :--- | :--- |
| **Online / Connecté / Actif** | `#4ade80` | Vert phosphore avec halo néon : `box-shadow: 0 0 5px #4ade80` |
| **Offline / Inactif** | `#666666` | Pastille mate neutre sans halo |
| **Alerte / Erreur / Danger** | `#ef4444` ou `#b91c1c` | Rouge signalétique (erreurs d'exécution, alertes système, statuts critiques) |
| **Action Destructive (Hover)** | `rgba(239, 68, 68, 0.25)` | Fond rouge translucide + texte `#f87171` |
| **Avertissement / Warning** | `#fef08a` / `#b91c1c` | Bandeau alerte fond rouge, texte jaune lumineux |
| **Code en ligne (Inline)** | `rgba(0, 0, 0, 0.3)` | Texte `--acc`, bordure `rgba(205, 204, 178, 0.2)` |
| **Terminal / Debug Pre** | `rgb(0, 29, 126)` ou `rgba(0, 0, 0, 0.4)` | Bleu cobalt console BIOS/Kernel ou noir fumé |
| **Ligne alternée / Bloc surligné** | `rgba(151, 177, 170, 0.15)` | Voile subtil teinté sauge (lignes de tableaux, éléments en sélection) |

---

## 4. Typographie & Hiérarchie Textuelle

### Font Stacks
- **Interface & Textes Généraux :**  
  `system-ui, -apple-system, "Segoe UI", Roboto, Ubuntu, Cantarell, "Noto Sans", Helvetica, Arial, sans-serif`  
  *(Polices système natives, ultra-rapides, nettes et sans fioritures).*
- **Données Techniques, Hashes, Clés, Code, Logs, Métriques :**  
  `ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, monospace`

### Règles de Typographie & Nommage de Code
- **Casse & Nommage Technique :** Privilégier les minuscules sobres ou la syntaxe technique directe (ex: `status: online`, `sync_rate`, `server_id`, `port: 8080`, `target_ip`).
- **Nommage de Variables & Identifiants (Code) :** Règle stricte "aucune voyelle et nom le plus court et direct possible" (disemvoweling). Raccourcir au maximum les identifiants tout en maintenant leur sens technique univoque (ex: `lbl`, `edt`, `btn`, `prst`, `dtls`, `st`, `cfg`, `rpc`, `cal`, `wnd`, `rc`, `bmp`, `msg`, `clnt`, `tm`, `idx`, `cnt`).
- **Sous-titres & Catégories :**  
  - Taille : `11px`
  - Style : `text-transform: uppercase`
  - Espacement : `letter-spacing: 0.5px`
  - Opacité : `0.7`
  - Poids : `600` (Semi-bold)
- **Titres de panneaux & Modules :** `13px - 14px`, `font-weight: 600`.
- **Textes d'entrées (Inputs) :** `14px` sur Desktop, forcé à `16px` sur Mobile pour empêcher le zoom automatique indésirable d'iOS.

---

## 5. Règles Géométriques & Structure

### Le Rayon de Courbure (`border-radius`)
- **Règle absolue :** `border-radius: 3px` sur quasiment tous les éléments (panneaux, fenêtres, rangées, boutons, champs, modales, conteneurs).
- **Exceptions tolérées :**
  - Pastilles d'état circulaires (`.status-dot`, `.indicator`) : `border-radius: 50%`
  - Badges numériques et pastilles compactes (`.badge`, `.tag`) : `border-radius: 10px`
- **Interdiction formelle :** Ne jamais utiliser de rayons supérieurs à 3px sur des boutons, conteneurs ou cartes (`rounded-lg`, `rounded-2xl`, etc. sont proscrits).

### Gestion des Bordures & Ombres
- **Bordures par défaut :** `1px solid var(--lnclr)` ou `1px solid var(--mid)`.
- **Ombres portées (Box Shadow) :** Interdites en règle générale. La profondeur est créée par la superposition de couches de couleurs (`--bg` > `--pane-bg` > `--mid`).
- **Seule exception d'ombre :** Le halo d'activité phosphoreux des pastilles d'état actives (`box-shadow: 0 0 5px #4ade80`).

### Animations & Transitions
- Durée rapide et sèche : `.15s ease` ou `.2s ease`. Pas d'effets rebondissants ("spring/bouncy"), pas de fondus lents.
- États de survol des boutons : `filter: brightness(1.15)` au lieu de changements complexes de teintes.

---

## 6. Composants Standards (Spécifications Précises)

### 1. Header d'Application
Bandeau sombre supérieur, compact et fixé.
```css
header {
    flex-shrink: 0;
    padding: 14px 18px;
    background: var(--mid);
    border-bottom: 1px solid var(--lnclr);
    color: var(--fg);
    font-weight: 600;
    font-size: 14px;
}
```

### 2. Panneau Principal (`.pane`, `.card`, `.module`)
Conteneur à fond ardoise sombre bordé de fil clair.
```css
.pane, .card, .module {
    background-color: var(--pane-bg);
    border: 1px solid var(--lnclr);
    border-radius: 3px;
    padding: 12px;
    color: var(--fg);
}
```

### 3. Rangée de Contrôle (`.row`)
Composant clé pour aligner libellé et champ d'action.
```css
.row {
    display: flex;
    align-items: center;
    gap: 8px;
    border: 1px solid var(--lnclr);
    border-radius: 3px;
    padding: 6px;
    margin-bottom: 8px;
}

.row label {
    white-space: nowrap;
    min-width: 80px;
    font-size: 13px;
    color: var(--fg);
}
```

### 4. Champs de Saisie (Inputs & Textareas)
```css
input[type="text"], input[type="password"], input[type="number"], textarea, select {
    background: var(--mid);
    color: var(--fg);
    border: 1px solid var(--lnclr);
    border-radius: 3px;
    padding: 8px 12px;
    font-family: inherit;
    font-size: 14px;
    outline: none;
    transition: border-color .15s ease;
}

input[type="text"]:focus, input[type="password"]:focus, input[type="number"]:focus, textarea:focus, select:focus {
    border-color: var(--acc);
    outline: 1px solid var(--acc);
}

input::placeholder, textarea::placeholder {
    color: var(--lnclr);
    opacity: 0.6;
}
```

### 5. Boutons Standards & Boutons d'Action
```css
button {
    background: var(--mid);
    color: var(--fg);
    border: 1px solid var(--mid);
    border-radius: 3px;
    padding: 8px 12px;
    font-size: 14px;
    cursor: pointer;
    white-space: nowrap;
    transition: filter .2s ease;
}

button:hover:not(:disabled) {
    filter: brightness(1.15);
}

button:disabled {
    opacity: 0.6;
    cursor: not-allowed;
}

/* Bouton ou élément de sélection actif */
button.active, .nav-item.active, .list-item.active, .item.active {
    background-color: var(--acc);
    color: var(--mid);
    border-color: var(--fg);
    font-weight: 700;
}
```
> **Convention de libellé console :** Pour les actions secondaires ou utilitaires, employer des crochets : `[ RUN ]`, `[ RAW ]`, `[ SYNC ]`, `[ CLEAR ]`, `[ SAVE ]`, `[ RESET ]`.

### 6. Badges & Pastilles de Statut
```css
/* Pastille d'état (service, connexion, processus) */
.status-dot, .indicator {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #666;
    flex-shrink: 0;
}

.status-dot.online, .status-dot.active, .indicator.active {
    background: #4ade80;
    box-shadow: 0 0 5px #4ade80;
}

/* Badge compteur ou étiquette sobre */
.badge {
    background: var(--mid);
    border: 1px solid var(--lnclr);
    border-radius: 3px;
    padding: 2px 6px;
    font-size: 11px;
    color: var(--fg);
}
```

### 7. Zone de Logs, Terminal & Flux (`.console`, `.terminal`, `.stream`, `.msgs`)
```css
.console-view, .log-stream, .terminal-window, .msgs {
    flex: 1;
    min-height: 250px;
    border: 1px solid var(--lnclr);
    border-radius: 3px;
    padding: 8px;
    overflow-y: auto;
    background-color: var(--mid);
    scrollbar-width: thin;
    font-family: ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, monospace;
}

/* Citation, encadré technique ou ligne d'annotation */
.callout-line, .log-quote, .msg-reply-quote {
    border-left: 2px solid var(--acc);
    padding-left: 6px;
    font-size: 12px;
    opacity: .75;
    margin-bottom: 2px;
}
```

---

## 7. Configuration Tailwind CSS Prête à l'Emploi

Pour les projets du studio utilisant Tailwind CSS, intégrer cette extension de thème dans `tailwind.config.js` :

```javascript
/** @type {import('tailwindcss').Config} */
module.exports = {
  theme: {
    extend: {
      colors: {
        studio: {
          bg: '#545e60',       // Surface fond générale
          pane: '#2a3536',     // Panneaux et cartes
          mid: '#162326',      // Zones sombres et inputs
          fg: '#ebe6ca',       // Texte clair parchemin
          line: '#cdccb2',     // Lignes et wireframe
          accent: '#97b1aa',   // Vert sauge interactif
          online: '#4ade80',   // Statut actif
          danger: '#ef4444',   // Erreurs et alertes
        }
      },
      borderRadius: {
        DEFAULT: '3px',
        studio: '3px',
      },
      fontFamily: {
        sans: ['system-ui', '-apple-system', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['ui-monospace', 'Cascadia Code', 'Source Code Pro', 'monospace'],
      }
    },
  },
}
```

---

## 8. Directives Strictes : Les DOs & DON'Ts

### Ce qui est REQUIS (DO) :
- Utiliser strictement les 5 couleurs fondatrices (`#545e60`, `#2a3536`, `#162326`, `#ebe6ca`, `#cdccb2`, `#97b1aa`).
- Conserver un rayon de courbure constant de `3px`.
- Délimiter les blocs fonctionnels par un contour fin de `1px`.
- Donner un style d'invitation console aux libellés techniques (`uppercase`, `tracking`, crochets).
- Privilégier une haute densité d'informations (paddings de 6 à 12px).
- Utiliser la police système pour le texte courant et monospace pour les chiffres/identifiants techniques.
- Appliquer la règle "aucune voyelle et nom le plus court et direct possible" (disemvoweling) pour les variables, fonctions, méthodes, constantes et identifiants de code.
- Rédiger des textes ultra-minimalistes, factuels, directs et concis (commentaires, README, commits, docs).

### Ce qui est STRICTEMENT INTERDIT (DON'T) :
- Aucun texte verbeux, bavard, scolaire ou "IA-like" dans les README, commits, documentations ou commentaires de code.
- Aucun dégradé violet / cyan / néon "SaaS générique" ou "AI gradient".
- Aucun bord arrondi doux (`rounded-xl`, `rounded-2xl`, formes de pilules géantes).
- Aucune ombre portée diffuse (`box-shadow: 0 20px 25px rgba(...)`).
- Aucun effet glassmorphism flou (`backdrop-blur-md`). Les surfaces sont solides et opaques.
- Pas d'illustrations plates vectorielles corporate ("Alegria style"). Si des médias sont requis, utiliser des icônes simples, du pixel art, du monospace ASCII ou des captures brutes.
- Pas de marges démesurées ou d'espaces vides superflus.

---

## 9. Snippet Système pour Agents IA (Instructions à copier-coller)

> Copiez le bloc ci-dessous directement dans vos fichiers `.cursorrules`, `.windsurfrules`, `CLAUDE.md`, ou dans le system prompt de vos agents IA pour leur faire appliquer automatiquement cette DA :

```markdown
# STUDIO UI/UX ARTISTIC DIRECTION INSTRUCTIONS

You are instructed to strictly follow the Studio's unified Art Direction (DA) for all UI, HTML, CSS, React, and Frontend code across any project.

### 1. Visual Theme
- The visual identity is utilitarian, clean, compact, engineering-driven, and technical.
- No modern SaaS tropes: NO large rounded corners, NO blurred glassmorphism, NO generic purple/blue gradients, NO giant whitespace.

### 2. Mandatory Color Palette
- Window/App Background: #545e60 (Muted slate khaki)
- Panels/Cards/Containers: #2a3536 (Carbon slate)
- Recessed/Inputs/Log Screens: #162326 (Deep dark obsidian petrol)
- Text/Foreground: #ebe6ca (Warm bone/parchment)
- Borders/Lines/Wireframe: #cdccb2 (Muted bone wireframe line)
- Interactive Accent: #97b1aa (Muted sage teal)
- Active/Online Status: #4ade80 with `box-shadow: 0 0 5px #4ade80`
- Danger/Alert/Error: #ef4444 / #b91c1c

### 3. Geometry & Layout
- Border Radius: ALWAYS `3px` everywhere (inputs, buttons, panes). Only exception is circular status dots (50%) and compact badges (10px). Never use rounded-lg or rounded-full for buttons or containers.
- Borders: Crisp 1px solid lines using #cdccb2 or #162326.
- Shadows: None, except the subtle 5px glow on the green online dot.
- Hover States: Use `filter: brightness(1.15)` on buttons.
- Transitions: Snappy and fast (`.15s ease`).

### 4. Typography, Naming & Components
- Font: Clean system font (`system-ui, sans-serif`) for UI; monospace for codes, hashes, metrics, and logs.
- Code Naming Convention: Shortest and most direct names possible without vowels (disemvoweling, for variables, functions, methods, e.g., `btn`, `wnd`, `lbl`, `edt`, `cfg`, `msg`, `rc`, `dtls`, `st`, `lnclr`, `clnt`, `ctx`, `tm`, `crt`, `swtch_vw`, `run_msg_lp`, `hndl_msg`).
- Tone & Documentation: Zero AI-like or academic fluff. Strictly minimalist, factual, and direct in all comments, READMEs, commits, and UI copy.
- Buttons & Labels: Compact, utilitarian. Use square bracket notation for actions where appropriate (e.g., `[ RUN ]`, `[ SYNC ]`, `[ SAVE ]`, `[ RESET ]`).
- Section Labels: 11px uppercase with `letter-spacing: 0.5px` and 0.7 opacity.
- High Information Density: Tight padding (6px to 12px), compact heights, strictly structured.
```
