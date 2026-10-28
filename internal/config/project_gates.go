package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/GasaTec/gasacli/internal/lifecycle"
	"gopkg.in/yaml.v3"
)

// ProjectGates defines commands GasaCLI runs itself after an agent finishes.
// Each command is an argv list; no shell expansion is performed.
type ProjectGates struct {
	Test  []string `yaml:"test"`
	Build []string `yaml:"build"`
	Lint  []string `yaml:"lint"`
}

// LoadProjectGates reads the optional trusted-project configuration at
// gasacli/config.yaml. Missing files mean no gates are configured.
func LoadProjectGates(project string) (ProjectGates, error) {
	directory := filepath.Join(project, "gasacli")
	dirInfo, dirErr := os.Lstat(directory)
	if errors.Is(dirErr, os.ErrNotExist) {
		return ProjectGates{}, nil
	}
	if dirErr != nil {
		return ProjectGates{}, fmt.Errorf("inspect project gate directory: %w", dirErr)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(dirInfo) {
		return ProjectGates{}, errors.New("project gate directory must be a regular directory")
	}
	path := filepath.Join(directory, "config.yaml")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ProjectGates{}, nil
	}
	if err != nil {
		return ProjectGates{}, fmt.Errorf("inspect project gates: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) {
		return ProjectGates{}, errors.New("project gates must be a regular file")
	}
	if info.Size() > 1<<20 {
		return ProjectGates{}, errors.New("project gate configuration exceeds 1 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return ProjectGates{}, fmt.Errorf("open project gates: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	var document yaml.Node
	if err = decoder.Decode(&document); err != nil {
		return ProjectGates{}, fmt.Errorf("decode project gates: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return ProjectGates{}, errors.New("project config must be a YAML mapping")
	}
	var gates ProjectGates
	seen := make(map[string]bool)
	root := document.Content[0]
	for index := 0; index+1 < len(root.Content); index += 2 {
		key, value := root.Content[index].Value, root.Content[index+1]
		if seen[key] {
			return ProjectGates{}, fmt.Errorf("duplicate project config key %q", key)
		}
		seen[key] = true
		var target *[]string
		switch key {
		case "test":
			target = &gates.Test
		case "build":
			target = &gates.Build
		case "lint":
			target = &gates.Lint
		default:
			continue
		}
		if err = value.Decode(target); err != nil {
			return ProjectGates{}, fmt.Errorf("decode project gate %s argv: %w", key, err)
		}
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return ProjectGates{}, fmt.Errorf("decode trailing project gates: %w", err)
		}
		return ProjectGates{}, errors.New("project gates must contain a single YAML document")
	}
	for name, argv := range map[string][]string{"test": gates.Test, "build": gates.Build, "lint": gates.Lint} {
		if len(argv) == 0 {
			continue
		}
		if len(argv) > 64 {
			return ProjectGates{}, fmt.Errorf("project gate %s has too many arguments", name)
		}
		if argv[0] == "" {
			return ProjectGates{}, fmt.Errorf("project gate %s requires an executable", name)
		}
		for _, arg := range argv {
			if arg == "" || len(arg) > 4096 || containsNUL(arg) {
				return ProjectGates{}, fmt.Errorf("project gate %s contains an invalid argument", name)
			}
		}
	}
	return gates, nil
}

func containsNUL(value string) bool {
	for _, r := range value {
		if r == 0 {
			return true
		}
	}
	return false
}
