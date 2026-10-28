package i18n

import (
	"reflect"
	"testing"
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

func TestConfigurePreferenceOrder(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	t.Setenv("LC_ALL", "pt_BR.UTF-8")
	Configure("Portuguese")
	if got := Language(); got != "pt-BR" {
		t.Fatalf("onboarding language=%q", got)
	}
	Configure("")
	if got := Language(); got != "en" {
		t.Fatalf("environment language=%q", got)
	}
}
