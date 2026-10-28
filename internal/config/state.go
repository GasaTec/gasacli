package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Onboarding OnboardingState `json:"onboarding"`
}

type OnboardingState struct {
	Completed   bool      `json:"completed"`
	Version     int       `json:"version"`
	CompletedAt time.Time `json:"completed_at"`
	CurrentStep int       `json:"current_step,omitempty"`
	Language    string    `json:"language,omitempty"`
	Permissions string    `json:"permissions,omitempty"`
	Dashboard   string    `json:"dashboard,omitempty"`
	Edition     string    `json:"edition,omitempty"`
}

func LoadState(home string) (State, error) {
	data, err := os.ReadFile(filepath.Join(home, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, nil
	}
	return state, nil
}

func SaveOnboardingState(home string, now time.Time) error {
	state, err := LoadState(home)
	if err != nil {
		return err
	}
	state.Onboarding.Completed = true
	state.Onboarding.Version = 1
	state.Onboarding.CompletedAt = now.UTC()
	return saveState(home, state)
}

func SaveOnboardingProgress(home string, onboarding OnboardingState) error {
	state, err := LoadState(home)
	if err != nil {
		return err
	}
	onboarding.Version = 2
	state.Onboarding = onboarding
	return saveState(home, state)
}

func saveState(home string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode onboarding state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(home, ".state-*")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(home, "state.json")); err != nil {
		return fmt.Errorf("replace onboarding state: %w", err)
	}
	return nil
}
