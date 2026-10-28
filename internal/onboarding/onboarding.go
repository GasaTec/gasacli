package onboarding

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/discovery"
)

const installTimeout = 20 * time.Minute

type AppDeps struct {
	LookPath LookupFunc
	Run      RunFunc
	Stat     func(string) (os.FileInfo, error)
	Getenv   func(string) string
}

func DetectApps(ctx context.Context, goos string, deps AppDeps) map[string]discovery.Result {
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.Run == nil {
		deps.Run = runCommand
	}
	if deps.Stat == nil {
		deps.Stat = os.Stat
	}
	if deps.Getenv == nil {
		deps.Getenv = os.Getenv
	}
	r := discovery.Result{ID: "obsidian", Name: "Obsidian", Status: discovery.NotFound, Note: "app; authentication not checked"}
	if goos == "windows" {
		if local := deps.Getenv("LOCALAPPDATA"); local != "" {
			p := filepath.Join(local, "Programs", "Obsidian", "Obsidian.exe")
			if _, e := deps.Stat(p); e == nil {
				r.Path = p
				r.Status = discovery.Available
				r.Version = "installed"
				return map[string]discovery.Result{"obsidian": r}
			}
		}
	}
	if goos == "darwin" {
		p := "/Applications/Obsidian.app"
		if _, e := deps.Stat(p); e == nil {
			r.Path = p
			r.Status = discovery.Available
			r.Version = "installed"
			return map[string]discovery.Result{"obsidian": r}
		}
	}
	if p, e := deps.LookPath("obsidian"); e == nil {
		r.Path = p
		r.Status = discovery.Available
		r.Version = "installed"
		return map[string]discovery.Result{"obsidian": r}
	}
	if goos == "linux" {
		if p, e := deps.LookPath("flatpak"); e == nil {
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, e = deps.Run(probeCtx, p, "info", "md.obsidian.Obsidian")
			cancel()
			if e == nil {
				r.Path = "flatpak:md.obsidian.Obsidian"
				r.Status = discovery.Available
				r.Version = "installed"
			}
		}
	}
	return map[string]discovery.Result{"obsidian": r}
}

type CommandRunner func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
type Options struct {
	In              io.Reader
	Input           *bufio.Reader
	Out             io.Writer
	InFile, OutFile *os.File
	Interactive     bool
	PlanOnly        bool
	GOOS            string
	Lookup          LookupFunc
	Run             RunFunc
	Command         CommandRunner
	Discover        func(context.Context) []discovery.Result
	Apps            map[string]discovery.Result
	Now             func() time.Time
	Keys            KeyReader
	// Numbered forces the line-based numbered prompt (used inside the chat, where the console
	// is in line mode and single keys would only arrive after Enter).
	Numbered    bool
	Stat        func(string) (os.FileInfo, error)
	Terminal    TerminalOpener
	KeysRestore func()
	Timeout     time.Duration
}

func defaults(o *Options) {
	if o.In == nil {
		o.In = os.Stdin
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Input == nil {
		o.Input = bufio.NewReader(o.In)
	}
	if o.InFile == nil {
		o.InFile = os.Stdin
	}
	if o.OutFile == nil {
		o.OutFile = os.Stdout
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Lookup == nil {
		o.Lookup = exec.LookPath
	}
	if o.Run == nil {
		o.Run = runCommand
	}
	if o.Command == nil {
		o.Command = realCommand
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Timeout <= 0 {
		o.Timeout = installTimeout
	}
	if o.Stat == nil {
		o.Stat = os.Stat
	}
	if o.Terminal == nil {
		o.Terminal = rawKeys
	}
	if o.Discover == nil {
		o.Discover = func(ctx context.Context) []discovery.Result {
			return discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
		}
	}
	if o.Apps == nil {
		detectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		o.Apps = DetectApps(detectCtx, normalizeOS(o.GOOS), AppDeps{LookPath: o.Lookup, Run: o.Run})
		cancel()
	}
}
func normalizeOS(osname string) string {
	if osname == "windows" {
		return "windows"
	}
	if osname == "darwin" {
		return "darwin"
	}
	return "linux"
}
func realCommand(ctx context.Context, path string, args []string, in io.Reader, out, errOut io.Writer) error {
	cmd := exec.Command(path, args...)
	if f, ok := in.(*os.File); ok {
		cmd.Stdin = f
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.WaitDelay = 3 * time.Second
	configureProcessTree(cmd)
	return runCommandTree(ctx, cmd)
}

func Run(ctx context.Context, o Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	defaults(&o)
	if o.KeysRestore != nil {
		defer o.KeysRestore()
	}
	agents := o.Discover(ctx)
	rows := make([]discovery.Result, 0, len(agents)+1)
	rows = append(rows, agents...)
	rows = append(rows, o.Apps["obsidian"])
	if err := writeTable(o.Out, rows); err != nil {
		return err
	}
	missing := missingItems(rows)
	if len(missing) == 0 {
		fmt.Fprintln(o.Out, "Não falta nenhum agente.")
		if app, ok := o.Apps["obsidian"]; ok && app.Status == discovery.NotFound {
			fmt.Fprintln(o.Out, "Obsidian é opcional (segundo cérebro).")
		}
	}
	if len(missing) == 0 {
		if o.PlanOnly {
			fmt.Fprintln(o.Out, "Para instalar, rode `gasa setup` fora do chat.")
		}
		return nil
	}
	defaultsSelected := map[string]bool{}
	for _, it := range missing {
		if it.ID == "claude" || it.ID == "codex" {
			defaultsSelected[it.ID] = true
		}
	}
	var selection map[string]bool
	if o.Keys != nil {
		var e error
		selection, e = (Selector{Reader: o.Keys, Out: o.Out}).Select(missing, defaultsSelected)
		if e != nil {
			return fmt.Errorf("selection canceled: %w", e)
		}
		if o.KeysRestore != nil {
			o.KeysRestore()
			o.KeysRestore = nil
		}
	} else if o.Interactive && o.Numbered {
		var e error
		selection, e = selectNumbers(o.Input, o.Out, missing, defaultsSelected)
		if e != nil {
			return e
		}
	} else if o.Interactive && o.InFile != nil && o.OutFile != nil {
		if selected, e := SelectRaw(missing, defaultsSelected, o.InFile, o.OutFile, o.Out, o.Terminal); e == nil {
			selection = selected
		} else if errors.Is(e, ErrTerminalUnavailable) {
			selection, e = selectNumbers(o.Input, o.Out, missing, defaultsSelected)
			if e != nil {
				return e
			}
		} else {
			return e
		}
	} else {
		selection = map[string]bool{}
		for _, it := range missing {
			selection[it.ID] = true
		}
		fmt.Fprintln(o.Out, "Suggested commands (no interactive terminal):")
	}
	var plan []planned
	for _, it := range missing {
		if !selection[it.ID] {
			continue
		}
		m, reason := chooseMethod(ctx, it, normalizeOS(o.GOOS), o.Lookup, o.Run)
		plan = append(plan, planned{item: it, method: m, reason: reason})
	}
	if o.PlanOnly {
		for _, p := range plan {
			fmt.Fprintf(o.Out, "\n%s — %s\nComando: %s\nOrigem oficial: %s\n", p.item.Name, p.method.Label, commandText(p.method), p.item.URL)
			if p.reason != "" {
				fmt.Fprintln(o.Out, "Motivo manual:", p.reason)
			}
			if p.item.Note != "" {
				fmt.Fprintln(o.Out, p.item.Note)
			}
		}
		if len(plan) == 0 {
			fmt.Fprintln(o.Out, "Nenhum item selecionado.")
		}
		fmt.Fprintln(o.Out, "Plano apenas; nenhuma instalacao sera executada.")
		fmt.Fprintln(o.Out, "Para instalar, rode `gasa setup` fora do chat.")
		return nil
	}
	if len(plan) == 0 {
		fmt.Fprintln(o.Out, "Nenhum item selecionado.")
		return nil
	}
	for _, p := range plan {
		fmt.Fprintf(o.Out, "\n%s — %s\nComando: %s\nOrigem oficial: %s\n", p.item.Name, p.method.Label, commandText(p.method), p.item.URL)
		if p.reason != "" {
			fmt.Fprintln(o.Out, "Motivo manual:", p.reason)
		}
		if p.item.Note != "" {
			fmt.Fprintln(o.Out, p.item.Note)
		}
	}
	if !o.Interactive {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Fprint(o.Out, "Run the automatic installs now? [y/N/s] ")
	line, err := o.Input.ReadString('\n')
	if err != nil && len(line) == 0 {
		return err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "s" {
		fmt.Fprintln(o.Out, "Instalações não executadas.")
		return nil
	}
	if answer == "s" {
		fmt.Fprintln(o.Out, "Comandos exibidos; nenhuma instalação executada.")
		return nil
	}
	for _, p := range plan {
		if !p.method.Auto {
			fmt.Fprintf(o.Out, "%s: instrução manual; não executada.\n", p.item.Name)
			continue
		}
		path, err := o.Lookup(p.method.Manager)
		if err != nil {
			fmt.Fprintf(o.Out, "%s: falhou antes de iniciar: %v\n", p.item.Name, err)
			continue
		}
		itemCtx, cancel := context.WithTimeout(ctx, o.Timeout)
		err = o.Command(itemCtx, path, p.method.Args, o.In, o.Out, o.Out)
		cancel()
		if err != nil {
			fmt.Fprintf(o.Out, "%s: falhou (%s). %v\n", p.item.Name, exitDescription(err), safeErr(err))
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		fmt.Fprintf(o.Out, "%s: comando concluído.\n", p.item.Name)
		verifyInstalled(ctx, p.item, o, o.Out)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	printNextSteps(o.Out, plan)
	return nil
}

func writeTable(w io.Writer, rows []discovery.Result) error {
	fmt.Fprintln(w, "Nome\tTipo\tStatus\tVersão\tNota")
	for _, r := range rows {
		kind := "agent"
		status := "missing"
		if r.Status == discovery.Available {
			status = "installed"
		}
		if r.ID == "obsidian" {
			kind = "app"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Name, kind, status, r.Version, r.Note)
	}
	return nil
}
func missingItems(rows []discovery.Result) []Item {
	all := Catalog()
	byID := map[string]discovery.Result{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	var missing []Item
	for _, it := range all {
		if r, ok := byID[it.ID]; !ok || r.Status != discovery.Available {
			missing = append(missing, it)
		}
	}
	return missing
}

type planned struct {
	item   Item
	method Method
	reason string
}

func commandText(m Method) string {
	if m.Auto {
		return m.Manager + " " + strings.Join(m.Args, " ")
	}
	return m.Instruction
}
func exitDescription(err error) string {
	var e *exec.ExitError
	if errors.As(err, &e) {
		return fmt.Sprintf("exit code %d", e.ExitCode())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelado"
	}
	return "erro"
}
func safeErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func verifyInstalled(ctx context.Context, it Item, o Options, out io.Writer) {
	if it.ID == "obsidian" {
		if app := DetectApps(ctx, normalizeOS(o.GOOS), AppDeps{LookPath: o.Lookup, Run: o.Run})["obsidian"]; app.Status == discovery.Available {
			fmt.Fprintf(out, "Verificação: Obsidian encontrado em %s\n", app.Path)
		} else {
			fmt.Fprintln(out, "Verificação: app não localizado no PATH atual.")
		}
		return
	}
	discoveryID := it.ID
	if discoveryID == "cursor" {
		discoveryID = "cursor-agent"
	}
	for _, agent := range discovery.DefaultCatalog() {
		if agent.ID != discoveryID {
			continue
		}
		result := discovery.Discover(ctx, []discovery.Agent{agent}, discovery.Deps{LookPath: o.Lookup, Run: o.Run})[0]
		if result.Status == discovery.Available {
			fmt.Fprintf(out, "Verificação: %s %s em %s\n", it.Name, result.Version, result.Path)
			return
		}
		break
	}
	fmt.Fprintf(out, "Verificação: %s ainda não encontrado no PATH atual.\n", it.Name)
	for _, p := range extraPaths(ctx, it, o) {
		if _, e := o.Stat(p); e == nil {
			fmt.Fprintf(out, "Instalado em %s; abra um novo terminal.\n", p)
			return
		}
	}
}
func extraPaths(ctx context.Context, it Item, o Options) []string {
	var roots []string
	npmCtx, npmCancel := context.WithTimeout(ctx, 5*time.Second)
	prefix, npmErr := o.Run(npmCtx, "npm", "prefix", "-g")
	npmCancel()
	if err := npmErr; err == nil && strings.TrimSpace(string(prefix)) != "" {
		root := strings.TrimSpace(string(prefix))
		roots = append(roots, root)
		if o.GOOS != "windows" {
			roots = append(roots, filepath.Join(root, "bin"))
		}
	}
	brewCtx, brewCancel := context.WithTimeout(ctx, 5*time.Second)
	prefix, brewErr := o.Run(brewCtx, "brew", "--prefix")
	brewCancel()
	if err := brewErr; err == nil && strings.TrimSpace(string(prefix)) != "" {
		roots = append(roots, filepath.Join(strings.TrimSpace(string(prefix)), "bin"))
	}
	if p := os.Getenv("APPDATA"); p != "" {
		roots = append(roots, filepath.Join(p, "npm"))
	}
	if p := os.Getenv("LOCALAPPDATA"); p != "" {
		roots = append(roots, filepath.Join(p, "Microsoft", "WinGet", "Links"))
	}
	if p, e := os.UserHomeDir(); e == nil {
		roots = append(roots, filepath.Join(p, ".local", "bin"))
	}
	var out []string
	for _, root := range roots {
		for _, b := range it.Binaries {
			out = append(out, filepath.Join(root, b), filepath.Join(root, b+".cmd"))
		}
	}
	return out
}
func printNextSteps(out io.Writer, plan []planned) {
	fmt.Fprintln(out, "Próximos passos:")
	for _, p := range plan {
		if p.item.Login != "" {
			fmt.Fprintf(out, "- %s: %s\n", p.item.Name, p.item.Login)
		}
	}
}
