package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/backup"
	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
	"golang.org/x/term"
)

func runBackup(ctx context.Context, args []string, out *os.File) error {
	if len(args) > 0 && args[0] == "backups" {
		home, e := config.Home()
		if e != nil {
			return e
		}
		entries, e := os.ReadDir(filepath.Join(home, "backups"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				fmt.Fprintln(out, filepath.Join(home, "backups", entry.Name()))
			}
		}
		return nil
	}
	if len(args) == 0 {
		args = []string{"backup"}
	}
	if args[0] == "backup" {
		args = args[1:]
		if len(args) > 0 && args[0] == "create" {
			args = args[1:]
		} else if len(args) > 0 && args[0] == "list" {
			return runBackup(ctx, append([]string{"backups"}, args[1:]...), out)
		}
	}
	if len(args) > 0 && args[0] == "restore" {
		return runRestore(ctx, args[1:], out)
	}
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var scope, pass string
	var encrypt bool
	fs.StringVar(&scope, "scope", "", "backup scope, may be repeated by comma")
	fs.BoolVar(&encrypt, "encrypt", false, "encrypt with a passphrase")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(1))
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	destination := filepath.Join(home, "backups", "gasacli-backup.zip")
	if fs.NArg() == 1 {
		destination = fs.Arg(0)
	}
	if encrypt && pass == "" {
		var err error
		pass, err = readPassphrase()
		if err != nil {
			return err
		}
	}
	scopes := []string(nil)
	if scope != "" {
		scopes = strings.Split(scope, ",")
	}
	if err := backup.Create(ctx, home, destination, backup.Options{Scopes: scopes, Passphrase: pass, Version: version}); err != nil {
		return err
	}
	fmt.Fprintln(out, destination)
	return nil
}

func runRestore(ctx context.Context, args []string, out *os.File) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var preview, yes bool
	var pass string
	fs.BoolVar(&preview, "preview", false, "show changes without applying them")
	fs.BoolVar(&yes, "yes", false, "confirm restore")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: gasa restore <file> [--preview]")
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	if strings.HasPrefix(readHeader(fs.Arg(0)), "GASA-BACKUP-1") {
		pass, err = readPassphrase()
		if err != nil {
			return err
		}
	}
	m, files, err := backup.Read(fs.Arg(0), pass)
	if err != nil {
		return err
	}
	if preview {
		for _, line := range backup.Preview(home, m, files) {
			fmt.Fprintln(out, line)
		}
		return nil
	}
	if !yes {
		return fmt.Errorf("restore requires --yes after reviewing --preview")
	}
	decision := permissions.New(home, trust.NewStore(home)).Decide(ctx, permissions.Request{Action: "write_vault"})
	if decision.Effect == permissions.Deny || decision.Effect == permissions.Ask && !yes {
		return fmt.Errorf("restore denied by permission profile (%s)", decision.Effect)
	}
	lines, err := backup.Restore(ctx, home, fs.Arg(0), pass, version)
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return err
}

func readHeader(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b := make([]byte, 13)
	n, _ := f.Read(b)
	return string(b[:n])
}
func readPassphrase() (string, error) {
	fmt.Fprint(os.Stderr, "Passphrase: ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}
