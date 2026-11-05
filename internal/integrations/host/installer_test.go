package host

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

type runnerFunc func(context.Context, Command) (Result, error)

func (f runnerFunc) Run(ctx context.Context, c Command) (Result, error) { return f(ctx, c) }
func fakeLookup(name string) (string, error) {
	if name == "npm" {
		return "/trusted/npm", nil
	}
	return "", exec.ErrNotFound
}
func TestInstallerConfirmationAndDurableUnknown(t *testing.T) {
	home := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	runner := runnerFunc(func(ctx context.Context, c Command) (Result, error) {
		close(started)
		select {
		case <-release:
			return Result{Started: true, ExitCode: 1}, errors.New("ambiguous failure")
		case <-ctx.Done():
			return Result{Started: true, ExitCode: -1}, ctx.Err()
		}
	})
	installer, err := NewWithRunner(home, runner, fakeLookup, func(context.Context) []Tool { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p, err := installer.Plan(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Start(context.Background(), p.ID, "different", "intent"); err == nil {
		t.Fatal("accepted a different plan hash")
	}
	job, err := installer.Start(context.Background(), p.ID, p.Hash, "intent")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	duplicate, err := installer.Start(context.Background(), p.ID, p.Hash, "intent")
	if err != nil || duplicate.ID != job.ID {
		t.Fatal("idempotent request did not preserve job")
	}
	restarted, err := NewWithRunner(home, runner, fakeLookup, func(context.Context) []Tool { return nil })
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.Get(job.ID)
	if err != nil || recovered.Status != "unknown" {
		t.Fatal("restart must expose unknown ownership")
	}
	if reconciled, err := restarted.Reconcile(context.Background(), job.ID); err == nil || reconciled.Status != "unknown" {
		t.Fatal("unverifiable original process was silently reconciled")
	}
	if _, err := restarted.Start(context.Background(), p.ID, p.Hash, "new-intent"); err == nil {
		t.Fatal("restarted server retried ambiguous installation")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, err := installer.Get(job.ID)
		if err == nil && got.Status == "unknown" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("installer failure lost unknown effects")
}
func TestManualInstallNeverStartsProcess(t *testing.T) {
	runner := runnerFunc(func(context.Context, Command) (Result, error) {
		t.Fatal("manual installer launched")
		return Result{}, nil
	})
	installer, err := NewWithRunner(t.TempDir(), runner, func(string) (string, error) { return "", exec.ErrNotFound }, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := installer.Plan(context.Background(), "orca")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Automatic {
		t.Fatal("Orca manual installation marked unattended")
	}
	if _, err := installer.Start(context.Background(), plan.ID, plan.Hash, "manual"); err == nil {
		t.Fatal("manual plan launched")
	}
}
