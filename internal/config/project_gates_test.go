package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadProjectGatesUsesArgvListsAndStrictYAML(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "gasacli", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test: [go, test, ./...]\nbuild: [go, build, ./cmd/gasa]\nlint: [golangci-lint, run]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gates, err := LoadProjectGates(project)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gates.Test, []string{"go", "test", "./..."}) || !reflect.DeepEqual(gates.Build, []string{"go", "build", "./cmd/gasa"}) || !reflect.DeepEqual(gates.Lint, []string{"golangci-lint", "run"}) {
		t.Fatalf("gates = %#v", gates)
	}
	if err = os.WriteFile(path, []byte("plugins: {github: true}\ntest: [go, test]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadProjectGates(project); err != nil {
		t.Fatalf("unrelated project configuration was rejected: %v", err)
	}
	if err = os.WriteFile(path, []byte("test: go test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadProjectGates(project); err == nil || !strings.Contains(err.Error(), "decode project gate test argv") {
		t.Fatalf("invalid gate argv error = %v", err)
	}
}

func TestLoadProjectGatesMissingAndInvalidCommand(t *testing.T) {
	project := t.TempDir()
	gates, err := LoadProjectGates(project)
	if err != nil || len(gates.Test)+len(gates.Build)+len(gates.Lint) != 0 {
		t.Fatalf("missing gates = %#v, %v", gates, err)
	}
	path := filepath.Join(project, "gasacli", "config.yaml")
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("test: ['', arg]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadProjectGates(project); err == nil {
		t.Fatal("empty executable was accepted")
	}
}
