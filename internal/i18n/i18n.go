// Package i18n provides the embedded product message catalog.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
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

// Configure selects the onboarding preference, then GASACLI_LANG, then the OS locale.
func Configure(onboardingLanguage string) {
	for _, candidate := range []string{onboardingLanguage, os.Getenv("GASACLI_LANG"), os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
		if strings.TrimSpace(candidate) != "" {
			SetLanguage(candidate)
			return
		}
	}
	SetLanguage("pt-BR")
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
	data, err := files.ReadFile("catalogs/" + Language() + ".json")
	if err != nil {
		return key
	}
	var messages catalog
	if json.Unmarshal(data, &messages) != nil {
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
	data, err := files.ReadFile("catalogs/" + language + ".json")
	if err != nil {
		return nil, err
	}
	var messages catalog
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}
