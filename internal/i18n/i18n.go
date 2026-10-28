// Package i18n provides the embedded product message catalog.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync/atomic"
)

//go:embed catalogs/*.json
var files embed.FS

type catalog map[string]string

var current atomic.Value

func init() { current.Store("pt-BR") }

// SetLanguage selects a supported locale. Unknown locale values use pt-BR.
func SetLanguage(language string) {
	language = normalize(language)
	if language != "en" && language != "pt-BR" {
		language = "pt-BR"
	}
	current.Store(language)
}

// Configure selects GASACLI_LANG, then the configured/onboarding preference, then the OS locale.
func Configure(onboardingLanguage string) {
	for _, candidate := range []string{os.Getenv("GASACLI_LANG"), onboardingLanguage, os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
		if strings.TrimSpace(candidate) != "" {
			SetLanguage(candidate)
			return
		}
	}
	SetLanguage("pt-BR")
}

// EffectiveLanguage returns the selected language and the setting that supplied it.
func EffectiveLanguage(configured, onboarding string) (language, source string) {
	for _, candidate := range []struct{ value, source string }{
		{os.Getenv("GASACLI_LANG"), "env"},
		{configured, "config"},
		{onboarding, "onboarding"},
		{os.Getenv("LC_ALL"), "os"},
		{os.Getenv("LC_MESSAGES"), "os"},
		{os.Getenv("LANG"), "os"},
	} {
		if selected := normalize(candidate.value); selected == "en" || selected == "pt-BR" {
			return selected, candidate.source
		}
	}
	return "pt-BR", "default"
}

func normalize(language string) string {
	language = strings.TrimSpace(strings.ToLower(strings.Split(language, ".")[0]))
	language = strings.ReplaceAll(language, "_", "-")
	if strings.HasPrefix(language, "pt") {
		return "pt-BR"
	}
	if strings.HasPrefix(language, "portugu") {
		return "pt-BR"
	}
	if strings.HasPrefix(language, "en") {
		return "en"
	}
	return ""
}

// Language returns the active catalog locale.
func Language() string { return current.Load().(string) }

// T looks up a message and applies fmt.Sprintf arguments when supplied.
func T(key string, args ...any) string {
	return TFor(Language(), key, args...)
}

// TFor looks up a message in a requested supported locale without changing the process locale.
func TFor(language, key string, args ...any) string {
	language = normalize(language)
	if language != "en" && language != "pt-BR" {
		language = "pt-BR"
	}
	messages, err := loadCatalog(files, language)
	if err != nil {
		return key
	}
	message, ok := messages[key]
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}

// CatalogKeys returns the keys in the selected catalog, primarily for validation.
func CatalogKeys(language string) ([]string, error) {
	language = normalize(language)
	if language != "en" && language != "pt-BR" {
		return nil, fmt.Errorf("unsupported language %q", language)
	}
	messages, err := loadCatalog(files, language)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func loadCatalog(source fs.FS, language string) (catalog, error) {
	entries, err := fs.ReadDir(source, "catalogs")
	if err != nil {
		return nil, err
	}
	merged := catalog{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !(name == language+".json" || strings.HasSuffix(name, "."+language+".json")) {
			continue
		}
		data, err := fs.ReadFile(source, "catalogs/"+name)
		if err != nil {
			return nil, err
		}
		var part catalog
		if err := json.Unmarshal(data, &part); err != nil {
			return nil, fmt.Errorf("decode catalog %s: %w", name, err)
		}
		for key, value := range part {
			if _, exists := merged[key]; exists {
				return nil, fmt.Errorf("duplicate catalog key %q in %s", key, name)
			}
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("catalog %s not found", language)
	}
	return merged, nil
}
