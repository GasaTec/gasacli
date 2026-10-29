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
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
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

func TestApprovalDialogKeyMatrix(t *testing.T) {
	keys := []struct {
		name string
		key  tea.KeyMsg
	}{{"enter", tea.KeyMsg{Type: tea.KeyEnter}}, {"esc", tea.KeyMsg{Type: tea.KeyEsc}}, {"space", tea.KeyMsg{Type: tea.KeySpace}}, {"up", tea.KeyMsg{Type: tea.KeyUp}}, {"down", tea.KeyMsg{Type: tea.KeyDown}}, {"left", tea.KeyMsg{Type: tea.KeyLeft}}, {"right", tea.KeyMsg{Type: tea.KeyRight}}, {"letter", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}}, {"y", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}}, {"n", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}}, {"tab", tea.KeyMsg{Type: tea.KeyTab}}}
	for _, test := range keys {
		t.Run(test.name, func(t *testing.T) {
			answer := make(chan ApprovalChoice, 1)
			model := newInteractiveModel(&Session{}, "/tmp/project", true)
			model.approval = &approvalPrompt{request: PermissionRequest{Action: "write_vault"}, answer: answer, choice: ApprovalDeny}
			updated, _ := model.Update(test.key)
			got := updated.(interactiveModel)
			switch test.name {
			case "up":
				if got.approval.choice != ApprovalRun {
					t.Fatal("Up did not move from Deny to Allow for this run")
				}
			case "enter", "space", "esc":
				want := ApprovalDeny
				if test.name == "space" {
					want = ApprovalDeny
				}
				if got.approval != nil || <-answer != want {
					t.Fatalf("%s did not choose Deny", test.name)
				}
			default:
				if got.approval.choice != ApprovalDeny || got.approval == nil {
					t.Fatalf("%q changed the default denial", test.name)
				}
			}
		})
	}

	model := newInteractiveModel(&Session{}, "/tmp/project", true)
	answer := make(chan ApprovalChoice, 1)
	model.approval = &approvalPrompt{answer: answer, choice: ApprovalDeny}
	for range 2 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
		model = updated.(interactiveModel)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if updated.(interactiveModel).approval != nil || <-answer != ApprovalOnce {
		t.Fatal("Up twice and Space did not allow once")
	}

	model = newInteractiveModel(&Session{}, "/tmp/project", true)
	answer = make(chan ApprovalChoice, 1)
	model.approval = &approvalPrompt{answer: answer, choice: ApprovalOnce}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(interactiveModel).approval != nil || <-answer != ApprovalDeny {
		t.Fatal("Esc did not deny an already selected approval")
	}
}
