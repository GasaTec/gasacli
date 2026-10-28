package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalDialogDefaultsToDenyAndAcceptsChoice(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	pinTestLanguage(t, "pt-BR")
	model := newInteractiveModel(&Session{}, "/tmp/project", true)
	answer := make(chan ApprovalChoice, 1)
	model.approval = &approvalPrompt{request: PermissionRequest{Agent: "tester", Action: "write_vault", Target: "vault/note.md", ExactCommand: "write note"}, answer: answer, choice: ApprovalDeny}
	model.width, model.height = 80, 24
	if !strings.Contains(model.View(), i18n.T("goal.approval.deny")) || !strings.Contains(model.View(), "write note") {
		t.Fatal("approval dialog is missing the default denial or exact command")
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(interactiveModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	if model.approval != nil || <-answer != ApprovalRun {
		t.Fatal("approval dialog did not send the accepted answer")
	}

	answer = make(chan ApprovalChoice, 1)
	model.approval = &approvalPrompt{request: PermissionRequest{Action: "write_vault"}, answer: answer, choice: ApprovalDeny}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(interactiveModel).approval != nil || <-answer != ApprovalDeny {
		t.Fatal("approval dialog did not default to denial")
	}
}

func TestApprovalDialogGolden80x24(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	pinTestLanguage(t, "pt-BR")
	model := newInteractiveModel(&Session{}, "/tmp/project", true)
	model.width, model.height = 80, 24
	model.approval = &approvalPrompt{request: PermissionRequest{Agent: "tester", Action: "write_vault", Target: "vault/note.md", ExactCommand: "write note"}, answer: make(chan ApprovalChoice, 1), choice: ApprovalDeny}
	got := normalizeInteractiveGolden(model.View())
	path := filepath.Join("testdata", "approval-80x24.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != normalizeInteractiveGolden(string(want)) {
		t.Fatalf("approval dialog differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
