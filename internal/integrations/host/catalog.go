// Package host adapts the existing trusted installation and discovery catalogs.
package host

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"

	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/onboarding"
)

type InstallMethod struct {
	Label       string `json:"label"`
	Automatic   bool   `json:"automatic"`
	Instruction string `json:"instruction,omitempty"`
	URL         string `json:"url,omitempty"`
}
type Tool struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	DiscoveryID  string          `json:"discoveryId,omitempty"`
	OnboardingID string          `json:"onboardingId,omitempty"`
	OrcaID       string          `json:"orcaId,omitempty"`
	HostID       string          `json:"hostId"`
	Status       string          `json:"status"`
	AuthStatus   string          `json:"authStatus"`
	Version      string          `json:"version,omitempty"`
	Path         string          `json:"path,omitempty"`
	Note         string          `json:"note,omitempty"`
	Selectable   bool            `json:"selectable"`
	Capabilities map[string]bool `json:"capabilities"`
	Methods      []InstallMethod `json:"methods"`
}

// HostID deliberately identifies a machine/user installation, not a runtime.
func HostID() string {
	name, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	sum := sha256.Sum256([]byte(name + "\x00" + home + "\x00" + runtime.GOOS))
	return fmt.Sprintf("local-%x", sum[:12])
}

func Catalog() []Tool {
	mappings := []struct{ id, discovery, onboarding, orca, name, kind string }{
		{"claude", "claude", "claude", "claude", "Claude Code", "agent"},
		{"codex", "codex", "codex", "codex", "Codex", "agent"},
		{"gemini", "gemini", "", "gemini", "Gemini CLI", "agent"},
		{"cursor", "cursor-agent", "cursor", "cursor", "Cursor CLI", "agent"},
		{"opencode", "opencode", "opencode", "opencode", "OpenCode", "agent"},
		{"kimi", "kimi", "kimi", "kimi", "Kimi Code CLI", "agent"},
		{"antigravity", "", "", "antigravity", "Antigravity", "agent"},
		{"orca", "orca", "orca", "", "Orca", "infrastructure"},
		{"obsidian", "", "obsidian", "", "Obsidian", "utility"},
		{"gh", "gh", "", "", "GitHub CLI", "utility"},
	}
	items := map[string]onboarding.Item{}
	for _, i := range onboarding.Catalog() {
		items[i.ID] = i
	}
	result := make([]Tool, 0, len(mappings))
	for _, m := range mappings {
		t := Tool{ID: m.id, Name: m.name, Kind: m.kind, DiscoveryID: m.discovery, OnboardingID: m.onboarding, OrcaID: m.orca, HostID: HostID(), Status: "unverified", AuthStatus: "unknown", Capabilities: map[string]bool{}, Methods: []InstallMethod{}}
		if m.id == "antigravity" {
			t.Status = "unavailable"
			t.Note = "Orca advertises an Antigravity agent; no verified standalone discovery or installation interface"
		}
		for _, method := range items[m.onboarding].Methods[runtime.GOOS] {
			// Interactive installers stay manual until their unattended contract is proven.
			t.Methods = append(t.Methods, InstallMethod{Label: method.Label, Automatic: method.Auto && method.Manager == "npm", Instruction: method.Instruction, URL: method.URL})
		}
		result = append(result, t)
	}
	return result
}

func Discover(ctx context.Context, deps discovery.Deps) []Tool {
	catalog := discovery.DefaultCatalog()
	for index := range catalog {
		if catalog[index].ID != "orca" {
			continue
		}
		selected := os.Getenv("ORCA_CLI_COMMAND")
		if selected == "" {
			switch {
			case os.Getenv("ORCA_DEV_REPO_ROOT") != "":
				selected = "orca-dev"
			case runtime.GOOS == "linux":
				selected = "orca-ide"
			default:
				selected = "orca"
			}
		}
		catalog[index].Binaries = []string{selected}
	}
	results := discovery.Discover(ctx, catalog, deps)
	byID := map[string]discovery.Result{}
	for _, r := range results {
		byID[r.ID] = r
	}
	tools := Catalog()
	for i := range tools {
		t := &tools[i]
		r, ok := byID[t.DiscoveryID]
		if !ok {
			continue
		}
		t.Status = string(r.Status)
		t.AuthStatus = string(r.Auth)
		t.Version = r.Version
		t.Path = r.Path
		t.Note = r.Note
		if r.Err != "" {
			t.Note = r.Err
		}
		// Native drivers currently implement only these providers. Orca readiness is
		// established separately by its versioned bridge, never by a binary's presence.
		supported := t.ID == "claude" || t.ID == "codex"
		t.Selectable = supported && r.Status == discovery.Ready
		t.Capabilities["execution"] = t.Selectable
		t.Capabilities["chat"] = t.Selectable
		t.Capabilities["resume"] = t.Selectable
		t.Capabilities["stop"] = false
	}
	return tools
}
