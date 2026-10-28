package onboarding

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Method struct {
	Label, Manager string
	Args           []string
	URL            string
	Auto           bool
	RequiresNode   string
	Instruction    string
}

type Item struct {
	ID, Name, Kind   string
	Binaries         []string
	URL, Login, Note string
	Methods          map[string][]Method
}

func auto(label, manager string, args ...string) Method {
	return Method{Label: label, Manager: manager, Args: args, Auto: true}
}
func manual(label, instruction, url string) Method {
	return Method{Label: label, Instruction: instruction, URL: url}
}
func nodeMethod(minimum string) Method {
	m := auto("npm", "npm", "install", "-g", "@moonshot-ai/kimi-code")
	m.RequiresNode = minimum
	return m
}

// Catalog stores every executable command as a fixed argument vector.
func Catalog() []Item {
	return []Item{
		{ID: "claude", Name: "Claude Code", Kind: "agent", Binaries: []string{"claude"}, URL: "https://code.claude.com/docs/en/setup", Login: "rodar claude", Methods: map[string][]Method{
			"windows": {auto("winget", "winget", "install", "--id", "Anthropic.ClaudeCode", "-e"), nodeClaude(), manual("manual", "irm https://claude.ai/install.ps1 | iex", "https://claude.ai/install.ps1")},
			"darwin":  {auto("brew", "brew", "install", "--cask", "claude-code"), manual("manual", "curl -fsSL https://claude.ai/install.sh | bash", "https://claude.ai/install.sh")},
			"linux":   {manual("manual", "curl -fsSL https://claude.ai/install.sh | bash", "https://claude.ai/install.sh"), nodeClaude()},
		}},
		{ID: "codex", Name: "Codex", Kind: "agent", Binaries: []string{"codex"}, URL: "https://github.com/openai/codex", Login: "rodar codex e escolher Sign in with ChatGPT", Methods: map[string][]Method{
			"windows": {auto("winget", "winget", "install", "--id", "OpenAI.Codex", "-e"), auto("npm", "npm", "install", "-g", "@openai/codex")},
			"darwin":  {auto("brew", "brew", "install", "--cask", "codex")},
			"linux":   {auto("npm", "npm", "install", "-g", "@openai/codex"), manual("manual", "curl -fsSL https://chatgpt.com/codex/install.sh | sh", "https://chatgpt.com/codex/install.sh")},
		}},
		{ID: "cursor", Name: "Cursor CLI", Kind: "agent", Binaries: []string{"cursor-agent", "agent"}, URL: "https://cursor.com/docs/cli/installation", Login: "agent login", Methods: map[string][]Method{
			"windows": {manual("manual", "irm 'https://cursor.com/install?win32=true' | iex", "https://cursor.com/install?win32=true")},
			"darwin":  {manual("manual", "curl https://cursor.com/install -fsS | bash", "https://cursor.com/install")},
			"linux":   {manual("manual", "curl https://cursor.com/install -fsS | bash", "https://cursor.com/install")},
		}},
		{ID: "opencode", Name: "OpenCode", Kind: "agent", Binaries: []string{"opencode"}, URL: "https://opencode.ai/docs/", Login: "rodar opencode e /connect", Note: "A v1 (npm/script/tap) pode conflitar com a v2 do Homebrew core; não misture métodos.", Methods: map[string][]Method{
			"windows": {auto("npm", "npm", "install", "-g", "opencode-ai"), manual("scoop", "scoop install opencode", "https://opencode.ai/docs/"), manual("manual", "curl -fsSL https://opencode.ai/install | bash", "https://opencode.ai/install")},
			"darwin":  {auto("npm", "npm", "install", "-g", "opencode-ai"), manual("manual", "curl -fsSL https://opencode.ai/install | bash", "https://opencode.ai/install")},
			"linux":   {auto("npm", "npm", "install", "-g", "opencode-ai"), manual("manual", "curl -fsSL https://opencode.ai/install | bash", "https://opencode.ai/install")},
		}},
		{ID: "orca", Name: "Orca", Kind: "agent", Binaries: []string{"orca"}, URL: "https://github.com/stablyai/orca", Login: "Settings → General → Orca CLI", Note: "Ative o CLI em Settings → General → Orca CLI. O oficial é github.com/stablyai/orca; existe outro binário homônimo.", Methods: map[string][]Method{
			"windows": {manual("manual", "Baixe orca-windows-setup.exe", "https://onorca.dev/download ou https://github.com/stablyai/orca/releases")},
			"darwin":  {auto("brew", "brew", "install", "--cask", "stablyai/orca/orca")},
			"linux":   {manual("manual", "AppImage ou AUR stably-orca-bin", "https://github.com/stablyai/orca/releases")},
		}},
		{ID: "kimi", Name: "Kimi Code CLI", Kind: "agent", Binaries: []string{"kimi"}, URL: "https://www.kimi.com/code/docs/en/kimi-code-cli/guides/getting-started", Login: "kimi e /login", Note: "No Windows, precisa do Git for Windows. Não use o antigo kimi-cli em Python.", Methods: map[string][]Method{
			"windows": {nodeMethod("22.19"), manual("manual", "irm https://code.kimi.com/kimi-code/install.ps1 | iex", "https://code.kimi.com/kimi-code/install.ps1")},
			"darwin":  {nodeMethod("22.19"), manual("manual", "curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash", "https://code.kimi.com/kimi-code/install.sh")},
			"linux":   {nodeMethod("22.19"), manual("manual", "curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash", "https://code.kimi.com/kimi-code/install.sh")},
		}},
		{ID: "obsidian", Name: "Obsidian", Kind: "app", URL: "https://obsidian.md/download", Methods: map[string][]Method{
			"windows": {auto("winget", "winget", "install", "--id", "Obsidian.Obsidian", "-e")},
			"darwin":  {auto("brew", "brew", "install", "--cask", "obsidian")},
			"linux":   {auto("flatpak", "flatpak", "install", "flathub", "md.obsidian.Obsidian"), manual("manual", "Baixe o Obsidian", "https://obsidian.md/download")},
		}},
	}
}

func nodeClaude() Method {
	m := auto("npm", "npm", "install", "-g", "@anthropic-ai/claude-code")
	m.RequiresNode = "22"
	return m
}
func currentOS() string { return normalizeOS(runtime.GOOS) }

type LookupFunc func(string) (string, error)
type RunFunc func(context.Context, string, ...string) ([]byte, error)

func chooseMethod(ctx context.Context, item Item, goos string, lookup LookupFunc, run RunFunc) (Method, string) {
	list := item.Methods[goos]
	for _, m := range list {
		if !m.Auto {
			continue
		}
		if _, err := lookup(m.Manager); err != nil {
			continue
		}
		if m.RequiresNode != "" {
			if _, err := lookup("node"); err != nil {
				continue
			}
			nodeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			out, err := run(nodeCtx, "node", "--version")
			cancel()
			if err != nil || !nodeAtLeast(string(out), m.RequiresNode) {
				continue
			}
		}
		return m, ""
	}
	for _, m := range list {
		if m.Auto || m.Instruction == "" {
			continue
		}
		reason := "nenhum método automático disponível"
		for _, candidate := range list {
			if !candidate.Auto {
				continue
			}
			if _, err := lookup(candidate.Manager); err != nil {
				reason = candidate.Manager + " não encontrado"
				break
			}
			if candidate.RequiresNode != "" {
				nodeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				out, err := run(nodeCtx, "node", "--version")
				cancel()
				if err != nil || !nodeAtLeast(string(out), candidate.RequiresNode) {
					reason = "Node " + strings.TrimSpace(string(out)) + " < " + candidate.RequiresNode
					break
				}
			}
		}
		return m, reason
	}
	return manual("manual", "Consulte a documentação oficial", item.URL), fmt.Sprintf("nenhum método disponível para %s", goos)
}

func nodeAtLeast(output, minimum string) bool {
	parse := func(value string) ([3]int, bool) {
		var v [3]int
		parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(value), "v"), ".")
		if len(parts) < 1 || len(parts) > 3 {
			return v, false
		}
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 {
				return v, false
			}
			v[i] = n
		}
		return v, true
	}
	got, ok := parse(output)
	if !ok {
		return false
	}
	want, ok := parse(minimum)
	if !ok {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return got[i] > want[i]
		}
	}
	return true
}
func runCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.Command(path, args...)
	cmd.WaitDelay = 3 * time.Second
	configureProcessTree(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := runCommandTree(ctx, cmd)
	return append(stdout.Bytes(), stderr.Bytes()...), err
}
