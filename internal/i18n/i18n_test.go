package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCatalogsHaveSameKeys(t *testing.T) {
	pt, err := CatalogKeys("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	en, err := CatalogKeys("en")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pt, en) {
		t.Fatalf("catalog keys differ: pt-BR=%v en=%v", pt, en)
	}
}

func TestPtBRCatalogsDoNotContainCommonUnaccentedWords(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("catalogs", "*.pt-BR.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no pt-BR catalog files found")
	}
	unaccented := []string{"execucao", "verificacao", "configuracao", "confirmacao", "padrao", "revisao", "sessao", "nao"}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var catalog map[string]string
			if err := json.Unmarshal(data, &catalog); err != nil {
				t.Fatal(err)
			}
			for key, value := range catalog {
				words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
					return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
				})
				for _, word := range words {
					for _, bad := range unaccented {
						if word == bad {
							t.Errorf("%s.%s contains unaccented word %q", filepath.Base(file), key, bad)
						}
					}
				}
			}
		})
	}
}

func TestConfigurePreferenceOrder(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	t.Setenv("LC_ALL", "pt_BR.UTF-8")
	Configure("Portuguese")
	if got := Language(); got != "en" {
		t.Fatalf("environment language=%q", got)
	}
	t.Setenv("GASACLI_LANG", "")
	Configure("Portuguese")
	if got := Language(); got != "pt-BR" {
		t.Fatalf("onboarding language=%q", got)
	}
}

func TestEffectiveLanguageReportsSource(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "pt_BR.UTF-8")
	if language, source := EffectiveLanguage("pt-BR", "en"); language != "en" || source != "env" {
		t.Fatalf("language=%s source=%s", language, source)
	}
	t.Setenv("GASACLI_LANG", "")
	if language, source := EffectiveLanguage("en", "pt-BR"); language != "en" || source != "config" {
		t.Fatalf("language=%s source=%s", language, source)
	}
	if language, source := EffectiveLanguage("", "pt-BR"); language != "pt-BR" || source != "onboarding" {
		t.Fatalf("language=%s source=%s", language, source)
	}
}

func TestLoadCatalogRejectsDuplicateKeysAcrossDomains(t *testing.T) {
	files := fstest.MapFS{
		"catalogs/en.json":        {Data: []byte(`{"shared":"core"}`)},
		"catalogs/tui.en.json":    {Data: []byte(`{"shared":"tui"}`)},
		"catalogs/tui.pt-BR.json": {Data: []byte(`{"shared":"tui"}`)},
	}
	if _, err := loadCatalog(files, "en"); err == nil {
		t.Fatal("duplicate key across catalog files was accepted")
	}
}
