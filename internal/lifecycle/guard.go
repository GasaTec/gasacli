package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidateHome canonicalizes home and rejects locations that are unsafe for a
// destructive operation. Every existing component is inspected without
// following links.
func ValidateHome(home string) (string, error) {
	return validateHome(home, false)
}

// ValidateEnsureHome applies the same path and identity checks as ValidateHome
// before config.Ensure writes anything. Missing and empty directories may be adopted.
func ValidateEnsureHome(home string) (string, error) {
	return validateHome(home, true)
}

func validateHome(home string, allowMissing bool) (string, error) {
	if home == "" {
		return "", fmt.Errorf("refusing data directory: path is empty")
	}
	if runtime.GOOS == "windows" {
		if err := validateWindowsSyntax(home); err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("resolve data directory: %w", err)
	}
	abs = filepath.Clean(abs)
	canonical, err := canonicalExistingPath(abs)
	if err != nil {
		return "", err
	}
	abs = canonical

	if err := rejectProtectedPath(abs); err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if allowMissing && os.IsNotExist(err) {
		return abs, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect data directory %s: %w", abs, err)
	}
	if isReparse(info) {
		return "", fmt.Errorf("refusing data directory %s: path is a link or reparse point", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("refusing data directory: %s is not a directory", abs)
	}
	markerPath := filepath.Join(abs, ".gasacli")
	markerInfo, markerErr := os.Lstat(markerPath)
	if markerErr == nil {
		if !markerInfo.Mode().IsRegular() || isReparse(markerInfo) {
			return "", fmt.Errorf("refusing data directory: marker must be a regular file")
		}
		data, err := os.ReadFile(markerPath)
		if err != nil {
			return "", fmt.Errorf("read GasaCLI identity marker: %w", err)
		}
		var marker struct {
			App    string `json:"app"`
			Schema int    `json:"schema"`
		}
		if err := json.Unmarshal(data, &marker); err != nil || marker.App != "gasacli" {
			return "", fmt.Errorf("refusing data directory: invalid .gasacli identity marker")
		}
	} else if !os.IsNotExist(markerErr) {
		return "", fmt.Errorf("inspect GasaCLI identity marker: %w", markerErr)
	} else if allowMissing {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return "", fmt.Errorf("list data directory %s: %w", abs, err)
		}
		if len(entries) == 0 {
			return abs, nil
		}
		if !isDefaultHome(abs) || !strictLegacyConfig(abs) || hasForeignApplicationSigns(abs) {
			return "", fmt.Errorf("GASACLI_HOME points to an existing directory that is not a GasaCLI data directory; choose an empty or new directory")
		}
	} else if !isDefaultHome(abs) || !strictLegacyConfig(abs) || hasForeignApplicationSigns(abs) {
		return "", fmt.Errorf("not a GasaCLI data directory (missing .gasacli marker); initialize with a new or empty directory via `GASACLI_HOME`, or remove it manually")
	}
	return abs, nil
}

// HasForeignApplicationSigns reports files commonly used to identify another
// application's working directory. It is shared by config adoption and reset.
func HasForeignApplicationSigns(home string) bool { return hasForeignApplicationSigns(home) }

func hasForeignApplicationSigns(home string) bool {
	for _, name := range []string{".git", ".hg", ".svn", ".orca", "package.json", "pyproject.toml", "Cargo.toml", "go.mod", "node_modules", "README", "README.md", "readme.txt"} {
		if _, err := os.Lstat(filepath.Join(home, name)); err == nil {
			return true
		}
	}
	for _, pattern := range []string{"*.sln", "*.SLN"} {
		matches, _ := filepath.Glob(filepath.Join(home, pattern))
		if len(matches) != 0 {
			return true
		}
	}
	if _, err := os.Lstat(filepath.Join(home, "logs")); err == nil {
		for _, name := range []string{"agents", "prompts", "trust.json"} {
			if _, err := os.Lstat(filepath.Join(home, name)); err == nil {
				return false
			}
		}
		return true
	}
	return false
}

// ContainedPath joins a name from the closed lifecycle list and proves that it
// remains beneath home.
func ContainedPath(home, name string) (string, error) { return containedPath(home, name) }

// IsReparsePoint reports whether info represents a link-like filesystem entry.
func IsReparsePoint(info os.FileInfo) bool { return isReparse(info) }

func containedPath(home, name string) (string, error) {
	p := filepath.Join(home, filepath.FromSlash(name))
	rel, err := filepath.Rel(home, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("refusing lifecycle entry %q: it escapes the data directory", name)
	}
	return p, nil
}

func rejectProtectedPath(home string) error {
	if isDefaultHome(home) {
		return nil
	}
	protected := []string{string(filepath.Separator)}
	if runtime.GOOS == "windows" {
		protected = append(protected, windowsProtectedPaths()...)
		if volume := filepath.VolumeName(home); volume != "" {
			protected = append(protected, volume+string(filepath.Separator))
		}
	} else {
		protected = append(protected, "/usr", "/bin", "/sbin", "/etc", "/var", "/opt", "/lib", "/lib64", "/Users", "/home", "/System", "/Library", "/Applications", "/Users/Shared", "/tmp")
		if roots, err := os.ReadDir(string(filepath.Separator)); err == nil {
			for _, entry := range roots {
				if strings.HasPrefix(strings.ToLower(entry.Name()), "lib") {
					protected = append(protected, filepath.Join(string(filepath.Separator), entry.Name()))
				}
			}
		}
		if h, err := os.UserHomeDir(); err == nil {
			protected = append(protected, h)
		}
	}
	for _, p := range protected {
		if p == "" {
			continue
		}
		resolved, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		resolved, err = canonicalExistingPath(filepath.Clean(resolved))
		if err != nil {
			resolved = filepath.Clean(resolved)
		}
		if samePath(home, resolved) || isAncestor(home, resolved) || (isStrictProtectedRoot(resolved) && isAncestor(resolved, home) && !isTempException(home)) {
			return fmt.Errorf("refusing data directory %s: it is equal to, above, or inside protected path %s", home, resolved)
		}
	}
	return nil
}

func isTempException(home string) bool { return isAncestor(filepath.Clean(os.TempDir()), home) }

func isDefaultHome(home string) bool {
	base, err := os.UserConfigDir()
	return err == nil && samePath(home, filepath.Join(base, "gasacli"))
}

func isStrictProtectedRoot(path string) bool {
	if runtime.GOOS == "windows" {
		for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData", "SystemRoot", "windir"} {
			if value := os.Getenv(key); value != "" && samePath(path, value) {
				return true
			}
		}
		if value := os.Getenv("ProgramFiles"); value != "" && samePath(path, filepath.Join(value, "WindowsApps")) {
			return true
		}
		return false
	}
	for _, root := range []string{"/usr", "/bin", "/sbin", "/etc", "/var", "/opt", "/lib", "/lib64", "/System", "/Library", "/Applications", "/Users/Shared"} {
		if samePath(path, root) {
			return true
		}
	}
	if filepath.Dir(path) == "/" && strings.HasPrefix(strings.ToLower(filepath.Base(path)), "lib") {
		return true
	}
	return false
}

func strictLegacyConfig(home string) bool {
	path := filepath.Join(home, "config.yaml")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	type webConfig struct {
		Mode     *string `yaml:"mode"`
		Host     *string `yaml:"host"`
		Port     *int    `yaml:"port"`
		Fallback *bool   `yaml:"fallback_port"`
	}
	var cfg struct {
		Web webConfig `yaml:"web"`
	}
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err := d.Decode(&cfg); err != nil {
		if err == io.EOF {
			return false
		}
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return false
	}
	if cfg.Web.Mode != nil && *cfg.Web.Mode != "auto" && *cfg.Web.Mode != "manual" && *cfg.Web.Mode != "always" {
		return false
	}
	if cfg.Web.Host != nil && *cfg.Web.Host != "127.0.0.1" && *cfg.Web.Host != "::1" && *cfg.Web.Host != "localhost" {
		return false
	}
	return cfg.Web.Port == nil || *cfg.Web.Port >= 1 && *cfg.Web.Port <= 65535
}

func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
func isAncestor(parent, child string) bool {
	r, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && r != "." && r != ".." && !filepath.IsAbs(r) && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
