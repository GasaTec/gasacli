package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/lifecycle"
)

var terminalInput = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var executablePath = os.Executable
var beforeRemoveOpen = func(string) error { return nil }

type resetOptions struct{ all, backups, dryRun, yes, uninstall bool }

func parseResetArgs(name string, args []string, allByDefault bool) (resetOptions, error) {
	opts := resetOptions{all: allByDefault}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.all, "all", opts.all, "include all GasaCLI data")
	fs.BoolVar(&opts.backups, "include-backups", false, "include user backups")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "show the plan without deleting")
	fs.BoolVar(&opts.yes, "yes", false, "skip the interactive confirmation")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return opts, nil
}

func runReset(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	opts, err := parseResetArgs("reset", args, false)
	if err != nil {
		return fmt.Errorf("usage: gasa reset [--all] [--include-backups] [--dry-run] [--yes]: %w", err)
	}
	return executeReset(ctx, opts, in, out, errOut)
}

func runUninstall(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	opts, err := parseResetArgs("uninstall", args, true)
	if err != nil {
		return fmt.Errorf("usage: gasa uninstall [--include-backups] [--dry-run] [--yes]: %w", err)
	}
	// Uninstall always includes the full GasaCLI payload.
	opts.all = true
	opts.uninstall = true
	home, homeErr := config.Home()
	if homeErr != nil {
		return fmt.Errorf("resolve GasaCLI data directory: %w", homeErr)
	}
	if _, statErr := os.Lstat(home); errors.Is(statErr, os.ErrNotExist) {
		fmt.Fprintln(out, "No GasaCLI data directory exists; nothing to delete.")
		path, err := executablePath()
		if err != nil {
			return fmt.Errorf("resolve executable for uninstall instructions: %w", err)
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("resolve executable symlink: %w", err)
		}
		return writeUninstallInstructions(out, path)
	} else if statErr != nil {
		return fmt.Errorf("inspect GasaCLI data directory: %w", statErr)
	}
	if err := executeReset(ctx, opts, in, out, errOut); err != nil {
		return err
	}
	path, err := executablePath()
	if err != nil {
		return fmt.Errorf("resolve executable for uninstall instructions: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("resolve executable symlink: %w", err)
	}
	if err := writeUninstallInstructions(out, path); err != nil {
		return err
	}
	return nil
}

func executeReset(ctx context.Context, opts resetOptions, in io.Reader, out, errOut io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	home, err := config.Home()
	if err != nil {
		return fmt.Errorf("resolve GasaCLI data directory: %w", err)
	}
	canonical, err := lifecycle.ValidateHome(home)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return fmt.Errorf("open data directory: %w", err)
	}
	defer root.Close()
	plan, err := makePlanRoot(root, opts)
	if err != nil {
		return err
	}
	if len(plan.items) == 0 {
		fmt.Fprintln(out, "Nothing to reset.")
		return nil
	}
	fmt.Fprintf(out, "Data directory: %s\n", canonical)
	for _, item := range plan.items {
		status := "kept"
		if item.remove {
			status = "delete"
		}
		if item.kind == "unexpected type" {
			fmt.Fprintf(out, "kept (unexpected type): %s\n", item.name)
			continue
		}
		if item.name == "backups" && opts.backups {
			fmt.Fprintf(out, "%s backups/ (user backups, irreversible, %d files, %s)\n", status, item.files, formatBytes(item.size))
		} else {
			fmt.Fprintf(out, "%s %s (%s, %d files, %s)\n", status, item.name, item.kind, item.files, formatBytes(item.size))
		}
		for _, name := range item.preview {
			fmt.Fprintf(out, "  %s\n", name)
		}
	}
	for _, name := range plan.unknown {
		fmt.Fprintf(out, "kept (not created by GasaCLI): %s\n", name)
	}
	if opts.all {
		folders := "agents/, prompts/, skills/, plugins/, and segundo-cerebro/"
		if opts.backups {
			folders += "; backups/ contains user backups that cannot be recovered"
		}
		fmt.Fprintf(out, "--all may include user files in %s.\n", folders)
	}
	if opts.backups {
		fmt.Fprintln(out, "backups/ contains user backups; deleting them is irreversible and they cannot be recovered.")
	}
	if opts.dryRun {
		return nil
	}
	if !opts.yes {
		if !terminalInput(in) {
			return fmt.Errorf("non-interactive: pass --yes to confirm")
		}
		word := "RESET"
		if opts.all || opts.backups {
			word = "DELETE"
		}
		fmt.Fprintf(out, "Type %s to continue: ", word)
		var response string
		if _, err := fmt.Fscanln(in, &response); err != nil {
			return fmt.Errorf("cancelled")
		}
		if strings.TrimSpace(response) != word {
			return fmt.Errorf("cancelled")
		}
	}

	var failures []string
	onboardingRemoved := false
	for _, item := range plan.items {
		if item.kind == "unexpected type" {
			failures = append(failures, item.name+": unexpected type; kept")
		}
	}
	for _, item := range plan.items {
		if !item.remove {
			continue
		}
		if err := ctx.Err(); err != nil {
			failures = append(failures, "operation cancelled: "+err.Error())
			break
		}
		if err := removeKnownRoot(root, item.name); err != nil {
			fmt.Fprintf(errOut, "error deleting %s: %v\n", item.name, err)
			failures = append(failures, item.name+": "+err.Error())
		} else {
			fmt.Fprintf(out, "deleted: %s\n", item.name)
			if item.name == "state.json" {
				onboardingRemoved = true
			}
		}
	}
	for _, name := range plan.unknown {
		fmt.Fprintf(out, "kept (not created by GasaCLI): %s\n", name)
	}
	rootDir, openErr := root.Open(".")
	var remaining []os.DirEntry
	var readErr error
	if openErr != nil {
		readErr = openErr
	} else {
		remaining, readErr = rootDir.ReadDir(-1)
		_ = rootDir.Close()
	}
	if readErr == nil && len(remaining) == 0 {
		_ = root.Close()
		if err := os.Remove(canonical); err == nil {
			fmt.Fprintln(out, "removed empty data directory")
		} else {
			failures = append(failures, "data directory: "+err.Error())
		}
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		failures = append(failures, "data directory: "+readErr.Error())
	}
	if len(failures) > 0 {
		return fmt.Errorf("reset completed with errors: %s", strings.Join(failures, "; "))
	}
	if onboardingRemoved && !opts.uninstall {
		fmt.Fprintln(out, "Run `gasa` to start onboarding again.")
	}
	return nil
}

type plannedItem struct {
	name, kind string
	size       int64
	files      int
	preview    []string
	remove     bool
}
type resetPlan struct {
	items   []plannedItem
	unknown []string
}

func makePlanRoot(root *os.Root, opts resetOptions) (resetPlan, error) {
	plan := resetPlan{}
	foldNames := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	known := make([]string, 0, len(lifecycle.Entries))
	for _, entry := range lifecycle.Entries {
		known = append(known, strings.TrimSuffix(entry.Name, "/"))
	}
	for _, entry := range lifecycle.Entries {
		name := strings.TrimSuffix(entry.Name, "/")
		info, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return plan, fmt.Errorf("inspect known entry %q: %w", name, err)
		}
		if isLink(info) {
			return plan, fmt.Errorf("refusing known entry %q: is a link or reparse point, not a directory; refusing to follow", name)
		}
		if lifecycle.IsDirectoryEntry(entry.Name) != info.IsDir() {
			plan.items = append(plan.items, plannedItem{name: name, kind: "unexpected type", remove: false})
			continue
		}
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		}
		size, files, preview, err := rootPathDetails(root, name, info)
		if err != nil {
			return plan, fmt.Errorf("size known entry %q: %w", name, err)
		}
		remove := (!entry.All && !entry.Backup) || (entry.All && opts.all) || (entry.Backup && opts.backups)
		plan.items = append(plan.items, plannedItem{name: name, kind: kind, size: size, files: files, preview: preview, remove: remove})
	}
	dir, err := root.Open(".")
	if err != nil {
		return plan, fmt.Errorf("open data directory for listing: %w", err)
	}
	entries, err := dir.ReadDir(-1)
	_ = dir.Close()
	if err != nil {
		return plan, fmt.Errorf("list data directory: %w", err)
	}
	for _, ent := range entries {
		if !knownNameMatch(ent.Name(), known, foldNames) {
			plan.unknown = append(plan.unknown, ent.Name())
		}
	}
	return plan, nil
}

func rootPathDetails(root *os.Root, name string, info os.FileInfo) (int64, int, []string, error) {
	if !info.IsDir() {
		return info.Size(), 1, nil, nil
	}
	dir, err := root.OpenRoot(name)
	if err != nil {
		return 0, 0, nil, err
	}
	defer dir.Close()
	file, err := dir.Open(".")
	if err != nil {
		return 0, 0, nil, err
	}
	children, err := file.ReadDir(-1)
	_ = file.Close()
	if err != nil {
		return 0, 0, nil, err
	}
	preview := make([]string, 0, min(10, len(children)))
	for i, child := range children {
		if i == 10 {
			break
		}
		preview = append(preview, child.Name())
	}
	var size int64
	files := 0
	for _, child := range children {
		childInfo, err := dir.Lstat(child.Name())
		if err != nil {
			return 0, 0, nil, err
		}
		if isLink(childInfo) {
			return 0, 0, nil, fmt.Errorf("contains a symbolic link, junction, or reparse point at %s", child.Name())
		}
		if childInfo.IsDir() {
			childSize, childFiles, _, err := rootPathDetails(dir, child.Name(), childInfo)
			if err != nil {
				return 0, 0, nil, err
			}
			size += childSize
			files += childFiles
		} else {
			size += childInfo.Size()
			files++
		}
	}
	return size, files, preview, nil
}

func knownNameMatch(name string, candidates []string, fold bool) bool {
	for _, candidate := range candidates {
		if fold {
			if strings.EqualFold(name, candidate) {
				return true
			}
		} else if name == candidate {
			return true
		}
	}
	return false
}

func removeKnownRoot(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if isLink(info) {
		return fmt.Errorf("refusing to follow symbolic link, junction, or reparse point")
	}
	if !info.IsDir() {
		return root.Remove(name)
	}
	if err := beforeRemoveOpen(name); err != nil {
		return err
	}
	dir, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer dir.Close()
	dirFile, err := dir.Open(".")
	if err != nil {
		return err
	}
	children, err := dirFile.ReadDir(-1)
	_ = dirFile.Close()
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := removeKnownRoot(dir, child.Name()); err != nil {
			return err
		}
	}
	return root.Remove(name)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	for _, unit := range units {
		value /= 1024
		if value < 1024 || unit == "TB" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%d bytes", n)
}

func writeUninstallInstructions(out io.Writer, executable string) error {
	if !filepath.IsAbs(executable) && !strings.HasPrefix(executable, "/") {
		if resolved, err := filepath.Abs(executable); err == nil {
			executable = resolved
		}
	}
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(executable)))
	var instruction string
	switch {
	case installedInWindowsProgramFiles(clean):
		instruction = "$env:GASA_UNINSTALL='1'; irm " + installerURL("install.ps1") + " | iex"
	case strings.Contains(clean, "node_modules") && strings.Contains(clean, "gasacli"):
		instruction = "npm rm -g gasacli"
	case strings.HasSuffix(clean, ".local/bin/gasa"):
		instruction = "GASA_UNINSTALL=1 curl -fsSL " + installerURL("install.sh") + " | sh"
	default:
		instruction = "Delete the binary at " + executable
	}
	_, err := fmt.Fprintln(out, instruction)
	return err
}

func installerURL(script string) string {
	ref := "main"
	if version != "dev" && version != "" {
		ref = "v" + version
	}
	return "https://raw.githubusercontent.com/GasaTec/gasacli/" + ref + "/scripts/" + script
}

func installedInWindowsProgramFiles(executable string) bool {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return false
	}
	root := strings.ToLower(filepath.ToSlash(filepath.Clean(localAppData)))
	installDir := strings.TrimRight(root, "/") + "/programs/gasacli/bin/"
	return strings.HasPrefix(executable, installDir)
}

func isLink(info os.FileInfo) bool { return lifecycle.IsReparsePoint(info) }
