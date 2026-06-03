package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

//go:embed themes/*.json
var builtinThemesFS embed.FS

type Theme struct {
	Name   string            `json:"name"`
	Label  string            `json:"label"`
	Colors map[string]string `json:"colors"`
}

// CSS returns safe CSS content with variables and Tailwind overrides for this theme.
func (t *Theme) CSS() template.CSS {
	var sb strings.Builder
	sb.WriteString(":root {\n")
	for k, v := range t.Colors {
		sb.WriteString("  --sm-" + k + ": " + v + ";\n")
	}
	sb.WriteString("}\n")
	sb.WriteString(`
/* Cards & surfaces */
.bg-white                          { background-color: var(--sm-card-bg)      !important; }
.bg-gray-50                        { background-color: var(--sm-thead-bg)     !important; }
.bg-gray-100                       { background-color: var(--sm-card-bg)      !important; }

/* Borders */
.border, .border-b, .border-t, .border-l, .border-r { border-color: var(--sm-card-border) !important; }
.divide-y > * + *, .divide-gray-50 > * + *, .divide-gray-100 > * + * { border-color: var(--sm-card-border) !important; }

/* Text */
.text-gray-900, .text-gray-800     { color: var(--sm-text-primary)   !important; }
.text-gray-700, .text-gray-600     { color: var(--sm-text-secondary) !important; }
.text-gray-500, .text-gray-400     { color: var(--sm-text-muted)     !important; }
.text-gray-300, .text-gray-200     { color: var(--sm-card-border)    !important; }
.text-blue-600                     { color: var(--sm-accent)          !important; }

/* Nav bar */
nav                                { background-color: var(--sm-nav-bg) !important; }
nav a, nav span, nav label         { color: var(--sm-nav-text) !important; }
nav a:hover                        { color: var(--sm-accent) !important; }
nav .font-bold                     { color: var(--sm-accent) !important; }
/* Nav dropdown — links live on card-bg, must use page text not nav-text */
a.dd-link                          { color: var(--sm-text-primary) !important; }
a.dd-link:hover                    { color: var(--sm-accent)        !important; opacity: 1 !important; }

/* Status dots */
.bg-green-400, .bg-green-500       { background-color: var(--sm-online)  !important; }
.bg-gray-200, .bg-gray-300         { background-color: var(--sm-offline) !important; }

/* Row hover */
.hover\:bg-blue-50\/30:hover       { background-color: var(--sm-row-hover) !important; }
.hover\:bg-gray-50:hover           { background-color: var(--sm-row-hover) !important; }

/* Primary action buttons */
.bg-blue-600                       { background-color: var(--sm-accent) !important; color: var(--sm-page-bg) !important; }
.hover\:bg-blue-700:hover          { background-color: var(--sm-accent) !important; filter: brightness(0.85); }

/* Info / accent badges */
.bg-blue-50                        { background-color: var(--sm-badge-info-bg)   !important; }
.bg-blue-100                       { background-color: var(--sm-badge-info-bg)   !important; }
.hover\:bg-blue-100:hover          { background-color: var(--sm-badge-info-bg)   !important; filter: brightness(0.92); }
.border-blue-200                   { border-color:     var(--sm-card-border)      !important; }
.text-blue-700                     { color:            var(--sm-badge-info-text) !important; }
.text-blue-600                     { color:            var(--sm-accent)          !important; }
.text-blue-500                     { color:            var(--sm-accent)          !important; }

/* Semantic status badges — all themes */
.bg-green-100                      { background-color: var(--sm-badge-success-bg)  !important; }
.text-green-800                    { color:            var(--sm-badge-success-txt) !important; }
.bg-red-100                        { background-color: var(--sm-badge-danger-bg)   !important; }
.bg-red-50                         { background-color: var(--sm-badge-danger-bg)   !important; }
.hover\:bg-red-100:hover           { background-color: var(--sm-badge-danger-bg)   !important; filter: brightness(0.92); }
.text-red-700, .text-red-800       { color:            var(--sm-badge-danger-txt)  !important; }
.text-red-600, .text-red-500       { color:            var(--sm-badge-danger-txt)  !important; }
.border-red-200                    { border-color:     var(--sm-badge-danger-bdr)  !important; }
.bg-orange-100                     { background-color: var(--sm-badge-orange-bg)   !important; }
.text-orange-800                   { color:            var(--sm-badge-orange-txt)  !important; }
.bg-yellow-100                     { background-color: var(--sm-badge-warning-bg)  !important; }
.text-yellow-800                   { color:            var(--sm-badge-warning-txt) !important; }
.bg-amber-50                       { background-color: var(--sm-badge-amber-bg)    !important; }
.text-amber-800                    { color:            var(--sm-badge-amber-txt)   !important; }

/* Purple → theme accent (mDNS badges, etc.) */
.bg-purple-50                      { background-color: var(--sm-badge-info-bg)   !important; }
.text-purple-700                   { color:            var(--sm-accent)          !important; }

/* Input / select */
input, select, textarea            { background-color: var(--sm-card-bg) !important; color: var(--sm-text-primary) !important; border-color: var(--sm-card-border) !important; }

/* Scrollbar */
::-webkit-scrollbar                { width: 6px; height: 6px; }
::-webkit-scrollbar-track          { background: var(--sm-page-bg); }
::-webkit-scrollbar-thumb          { background: var(--sm-card-border); border-radius: 3px; }
`)
	return template.CSS(sb.String())
}

type ThemeManager struct {
	dataDir string
	themes  map[string]*Theme
	active  string
}

func NewThemeManager(dataDir string) *ThemeManager {
	tm := &ThemeManager{
		dataDir: dataDir,
		themes:  make(map[string]*Theme),
		active:  "light",
	}
	tm.loadBuiltins()
	tm.loadCustom()
	tm.loadActive()
	return tm
}

func (tm *ThemeManager) loadBuiltins() {
	entries, _ := builtinThemesFS.ReadDir("themes")
	for _, e := range entries {
		data, err := builtinThemesFS.ReadFile("themes/" + e.Name())
		if err != nil {
			continue
		}
		var t Theme
		if json.Unmarshal(data, &t) == nil {
			tm.themes[t.Name] = &t
		}
	}
}

func (tm *ThemeManager) loadCustom() {
	dir := filepath.Join(tm.dataDir, "themes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var t Theme
		if json.Unmarshal(data, &t) == nil {
			tm.themes[t.Name] = &t
		}
	}
}

func (tm *ThemeManager) loadActive() {
	data, err := os.ReadFile(filepath.Join(tm.dataDir, "theme.json"))
	if err != nil {
		return
	}
	var s struct {
		Active string `json:"active"`
	}
	if json.Unmarshal(data, &s) == nil && s.Active != "" {
		if _, ok := tm.themes[s.Active]; ok {
			tm.active = s.Active
		}
	}
}

func (tm *ThemeManager) SetActive(name string) {
	if _, ok := tm.themes[name]; !ok {
		return
	}
	tm.active = name
	data, _ := json.Marshal(map[string]string{"active": name})
	os.WriteFile(filepath.Join(tm.dataDir, "theme.json"), data, 0644)
}

func (tm *ThemeManager) Active() *Theme {
	if t, ok := tm.themes[tm.active]; ok {
		return t
	}
	return tm.themes["light"]
}

func (tm *ThemeManager) All() []*Theme {
	out := make([]*Theme, 0, len(tm.themes))
	for _, t := range tm.themes {
		out = append(out, t)
	}
	return out
}
