package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOnboardingState(t *testing.T) {
	home := t.TempDir()
	state, err := LoadState(home)
	if err != nil || state.Onboarding.Completed {
		t.Fatalf("initial=%+v err=%v", state, err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if err := SaveOnboardingState(home, now); err != nil {
		t.Fatal(err)
	}
	state, err = LoadState(home)
	if err != nil || !state.Onboarding.Completed || state.Onboarding.Version != 1 || !state.Onboarding.CompletedAt.Equal(now) {
		t.Fatalf("saved=%+v err=%v", state, err)
	}
	if files, err := os.ReadDir(home); err != nil || len(files) != 1 || files[0].Name() != "state.json" {
		t.Fatalf("non-atomic leftovers: %v %v", files, err)
	}
}

func TestCorruptStateIsNotCompletedAndNotDeleted(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.json")
	if err := os.WriteFile(path, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := LoadState(home)
	if err != nil || state.Onboarding.Completed {
		t.Fatalf("corrupt=%+v err=%v", state, err)
	}
	data, _ := os.ReadFile(path)
	if strings.TrimSpace(string(data)) != "{bad" {
		t.Fatal("corrupt state was changed")
	}
}
