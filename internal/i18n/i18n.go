package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed locales/*.json
var localeFS embed.FS

// Bundle holds all loaded translations.
type Bundle struct {
	langs    map[string]map[string]string
	fallback string
}

// New loads all *.json files from the embedded locales directory.
func New() (*Bundle, error) {
	b := &Bundle{
		langs:    make(map[string]map[string]string),
		fallback: "de",
	}
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(e.Name(), ".json")
		data, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("i18n: read %s: %w", e.Name(), err)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("i18n: parse %s: %w", e.Name(), err)
		}
		b.langs[lang] = m
	}
	return b, nil
}

// T returns the translation for key in lang, falling back to "de", then the key itself.
func (b *Bundle) T(lang, key string) string {
	if m, ok := b.langs[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	// Fallback to default language
	if lang != b.fallback {
		if m, ok := b.langs[b.fallback]; ok {
			if v, ok := m[key]; ok {
				return v
			}
		}
	}
	return key // Return key as last resort so missing strings are visible
}

// Tf returns T(lang, key) formatted with args via fmt.Sprintf.
func (b *Bundle) Tf(lang, key string, args ...any) string {
	s := b.T(lang, key)
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Languages returns a sorted list of available language codes.
func (b *Bundle) Languages() []string {
	langs := make([]string, 0, len(b.langs))
	for k := range b.langs {
		langs = append(langs, k)
	}
	sort.Strings(langs)
	return langs
}

// TimeAgo returns a human-readable relative time string in the given language.
func (b *Bundle) TimeAgo(lang string, t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 30*time.Second:
		return b.T(lang, "time.justnow")
	case d < time.Hour:
		return b.Tf(lang, "time.minutes", int(d.Minutes()))
	case d < 24*time.Hour:
		return b.Tf(lang, "time.hours", int(d.Hours()))
	case d < 7*24*time.Hour:
		return b.Tf(lang, "time.days", int(d.Hours()/24))
	default:
		return b.Tf(lang, "time.weeks", int(d.Hours()/(24*7)))
	}
}
