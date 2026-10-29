package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
)

// sidebarSnapshot is read on a worker, never during rendering: discovery can
// invoke external CLIs. Project entries are restricted to the trust store.
type sidebarSnapshot struct {
	Projects []sidebarProject
	Agents   []sidebarAgent
	Err      error
}

type sidebarProject struct {
	Path, Name string
	Active     bool
}
type sidebarAgent struct {
	ID, Name string
	Status   discovery.Status
	Auth     discovery.AuthStatus
	Role     string
}

func loadSidebarSnapshot(s *Session, active string) sidebarSnapshot {
	var snapshot sidebarSnapshot
	if s.TrustStore == nil {
		snapshot.Err = fmt.Errorf("project trust store unavailable")
		return snapshot
	}
	paths, err := s.TrustStore.TrustedProjects()
	if err != nil {
		snapshot.Err = err
		return snapshot
	}
	for _, path := range paths {
		snapshot.Projects = append(snapshot.Projects, sidebarProject{Path: path, Name: filepath.Base(path), Active: path == active})
	}
	cfg, err := config.LoadFrom(s.ConfigHome)
	if err != nil {
		snapshot.Err = err
		return snapshot
	}
	if s.DiscoveryResults == nil {
		return snapshot
	}
	for _, result := range discoveryResults(s.DiscoveryResults()) {
		var roles []string
		if strings.EqualFold(cfg.Agents.Developer, result.ID) {
			roles = append(roles, "developer")
		}
		if strings.EqualFold(cfg.Agents.Reviewer, result.ID) {
			roles = append(roles, "reviewer")
		}
		// An explicitly configured provider remains useful to show when its
		// executable is missing: otherwise its configuration silently vanishes.
		if result.Status == discovery.Missing && len(roles) == 0 {
			continue
		}
		snapshot.Agents = append(snapshot.Agents, sidebarAgent{ID: result.ID, Name: result.Name, Status: result.Status, Auth: result.Auth, Role: strings.Join(roles, "/")})
	}
	return snapshot
}
