package host

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/onboarding"
)

type Plan struct {
	ID           string    `json:"id"`
	Hash         string    `json:"hash"`
	HostID       string    `json:"hostId"`
	ToolID       string    `json:"toolId"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Executable   string    `json:"executable,omitempty"`
	Args         []string  `json:"args"`
	Method       string    `json:"method"`
	Automatic    bool      `json:"automatic"`
	Instructions string    `json:"instructions,omitempty"`
	URL          string    `json:"url"`
	Prerequisite string    `json:"prerequisite,omitempty"`
}
type Job struct {
	ID              string    `json:"id"`
	PlanID          string    `json:"planId"`
	ToolID          string    `json:"toolId"`
	HostID          string    `json:"hostId"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	ExitCode        int       `json:"exitCode"`
	Error           string    `json:"error,omitempty"`
	IdempotencyKey  string    `json:"-"`
	PlanHash        string    `json:"-"`
	ProcessID       int       `json:"processId,omitempty"`
	ProcessIdentity string    `json:"processIdentity,omitempty"`
	OwnerID         int       `json:"ownerId,omitempty"`
	OwnerIdentity   string    `json:"ownerIdentity,omitempty"`
	ProcessFinished bool      `json:"processFinished"`
}
type storedJob struct {
	Job  Job    `json:"job"`
	Key  string `json:"key"`
	Hash string `json:"hash"`
}
type Installer struct {
	mu       sync.Mutex
	dir      string
	hostID   string
	runner   Runner
	lookup   func(string) (string, error)
	discover func(context.Context) []Tool
	cancels  map[string]context.CancelFunc
}

func New(home string) (*Installer, error) {
	return NewWithRunner(home, ExecRunner{}, exec.LookPath, nil)
}
func NewWithRunner(home string, runner Runner, lookup func(string) (string, error), discover func(context.Context) []Tool) (*Installer, error) {
	if home == "" || runner == nil || lookup == nil {
		return nil, errors.New("installer dependencies required")
	}
	dir := filepath.Join(home, "office-install")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if discover == nil {
		discover = func(ctx context.Context) []Tool { return Discover(ctx, discovery.Deps{}) }
	}
	return &Installer{dir: dir, hostID: HostID(), runner: runner, lookup: lookup, discover: discover, cancels: map[string]context.CancelFunc{}}, nil
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func validID(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func hashPlan(p Plan) string {
	p.Hash = ""
	p.ID = ""
	p.ExpiresAt = time.Time{}
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (i *Installer) Plan(ctx context.Context, toolID string) (Plan, error) {
	p, err := i.buildPlan(ctx, toolID)
	if err != nil {
		return p, err
	}
	p.ID = randomID()
	p.Hash = hashPlan(p)
	p.ExpiresAt = time.Now().UTC().Add(10 * time.Minute)
	i.mu.Lock()
	defer i.mu.Unlock()
	err = i.write("plan-"+p.ID+".json", p)
	return p, err
}
func (i *Installer) buildPlan(ctx context.Context, toolID string) (Plan, error) {
	var item onboarding.Item
	for _, candidate := range onboarding.Catalog() {
		if candidate.ID == toolID {
			item = candidate
			break
		}
	}
	if item.ID == "" {
		return Plan{}, errors.New("no verified installation catalog entry")
	}
	p := Plan{HostID: i.hostID, ToolID: toolID, URL: item.URL, Args: []string{}, Instructions: item.Note}
	// Only npm's fixed install vector is supported unattended. OS installers can
	// elevate or prompt; present their existing catalog instructions instead.
	for _, m := range item.Methods[runtime.GOOS] {
		if !m.Auto || m.Manager != "npm" {
			continue
		}
		if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
			continue
		}
		path, err := i.lookup(m.Manager)
		if err != nil {
			continue
		}
		if m.RequiresNode != "" {
			node, err := i.lookup("node")
			if err != nil {
				continue
			}
			result, err := i.runner.Run(ctx, Command{Executable: node, Args: []string{"--version"}, Timeout: 5 * time.Second})
			if err != nil || result.ExitCode != 0 || !nodeAtLeast(result.Stdout, m.RequiresNode) {
				continue
			}
		}
		p.Executable = path
		p.Args = append([]string(nil), m.Args...)
		// Windows npm.cmd is a shell script. Run npm's installed JavaScript
		// entrypoint with Node rather than passing a command through cmd.exe.
		if strings.EqualFold(filepath.Ext(path), ".cmd") || strings.EqualFold(filepath.Ext(path), ".bat") {
			node, lookupErr := i.lookup("node")
			script := filepath.Join(filepath.Dir(path), "node_modules", "npm", "bin", "npm-cli.js")
			info, statErr := os.Stat(script)
			if lookupErr != nil || statErr != nil || !info.Mode().IsRegular() {
				continue
			}
			p.Executable = node
			p.Args = append([]string{script}, m.Args...)
		}
		p.Method = m.Label
		p.Automatic = true
		p.Prerequisite = m.RequiresNode
		p.Instructions = "Instalação global no escopo configurado do npm; não instala dependências do sistema e não faz login."
		return p, nil
	}
	for _, m := range item.Methods[runtime.GOOS] {
		if !m.Auto && m.Instruction != "" {
			p.Method = m.Label
			p.Instructions = m.Instruction
			p.URL = m.URL
			return p, nil
		}
	}
	p.Method = "manual"
	p.Instructions = "Consulte a documentação oficial. Nenhum método automático não interativo disponível neste host."
	return p, nil
}
func nodeAtLeast(value, minimum string) bool {
	parse := func(s string) ([3]int, bool) {
		var out [3]int
		parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
		if len(parts) > 3 {
			return out, false
		}
		for j, p := range parts {
			v, e := strconv.Atoi(p)
			if e != nil || v < 0 {
				return out, false
			}
			out[j] = v
		}
		return out, true
	}
	a, ok := parse(value)
	if !ok {
		return false
	}
	b, ok := parse(minimum)
	if !ok {
		return false
	}
	for j := range a {
		if a[j] != b[j] {
			return a[j] > b[j]
		}
	}
	return true
}

// Start requires the exact plan hash shown to the human. The request context is
// used for preflight only; closing an HTTP connection never retries/cancels a job.
func (i *Installer) Start(ctx context.Context, planID, confirmedHash, key string) (Job, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	if !validID(planID) || len(key) < 1 || len(key) > 200 {
		return Job{}, errors.New("invalid installation request")
	}
	var p Plan
	if err := i.read("plan-"+planID+".json", &p); err != nil {
		return Job{}, err
	}
	if confirmedHash == "" || confirmedHash != p.Hash || hashPlan(p) != p.Hash || p.HostID != i.hostID {
		return Job{}, errors.New("plan or host confirmation mismatch")
	}
	jobs, err := i.jobs()
	if err != nil {
		return Job{}, err
	}
	for _, j := range jobs {
		if j.IdempotencyKey == key {
			if j.PlanID != planID || j.PlanHash != confirmedHash {
				return Job{}, errors.New("idempotency key conflicts with another plan")
			}
			return j, nil
		}
	}
	if time.Now().After(p.ExpiresAt) {
		return Job{}, errors.New("installation plan expired")
	}
	if !p.Automatic {
		return Job{}, errors.New("manual installation required; use official instructions")
	}
	fresh, err := i.buildPlan(ctx, p.ToolID)
	if err != nil {
		return Job{}, err
	}
	if hashPlan(fresh) != p.Hash {
		return Job{}, errors.New("installation method or prerequisites changed; create a new plan")
	}
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	for _, j := range jobs {
		if j.HostID == p.HostID && j.ToolID == p.ToolID && (j.Status == "installing" || j.Status == "verifying" || j.Status == "unknown" || j.Status == "cancel_requested") {
			return Job{}, errors.New("installation requires observation or reconciliation before another attempt")
		}
	}
	// O_EXCL provides a host/tool fence across independent server processes.
	lock := filepath.Join(i.dir, "lock-"+p.HostID+"-"+p.ToolID)
	file, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Job{}, errors.New("installation is locked; reconcile the previous job")
	}
	_ = file.Close()
	now := time.Now().UTC()
	j := Job{ID: randomID(), PlanID: p.ID, ToolID: p.ToolID, HostID: p.HostID, Status: "installing", CreatedAt: now, UpdatedAt: now, ExitCode: -1, IdempotencyKey: key, PlanHash: p.Hash}
	if err = i.save(j); err != nil {
		_ = os.Remove(lock)
		return Job{}, err
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	i.cancels[j.ID] = cancel
	go i.execute(jobCtx, cancel, p, j, lock)
	return j, nil
}
func (i *Installer) execute(ctx context.Context, cancel context.CancelFunc, p Plan, j Job, lock string) {
	defer cancel()
	result, err := i.runner.Run(ctx, Command{Executable: p.Executable, Args: p.Args, Dir: i.dir, Timeout: 20 * time.Minute, OnStart: func(pid int) error {
		identity, err := processIdentity(pid)
		if err != nil {
			return errors.New("could not establish installer process identity")
		}
		ownerIdentity, err := processIdentity(os.Getpid())
		if err != nil {
			return errors.New("could not establish installer owner identity")
		}
		i.mu.Lock()
		defer i.mu.Unlock()
		j.ProcessID = pid
		j.ProcessIdentity = identity
		j.OwnerID = os.Getpid()
		j.OwnerIdentity = ownerIdentity
		return i.save(j)
	}})
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.cancels, j.ID)
	j.ExitCode = result.ExitCode
	if j.ProcessID > 0 {
		j.ProcessFinished = processTreeAbsent(j.ProcessID)
	}
	j.UpdatedAt = time.Now().UTC()
	if err != nil || result.ExitCode != 0 {
		j.Status = "failed"
		j.Error = "Installer failed; inspect the package manager and run discovery before retrying."
		if result.Started {
			j.Status = "unknown"
			j.Error = "Installer started; effects require reconciliation before retrying."
		}
		if !result.Started {
			_ = os.Remove(lock)
		}
		_ = i.save(j)
		return
	}
	j.Status = "verifying"
	if err := i.save(j); err != nil {
		return
	}
	// Probe after execution using the same discovery contract; successful exit is
	// not installation/authentication proof. No raw installer output is persisted.
	i.mu.Unlock()
	tools := i.discover(ctx)
	i.mu.Lock()
	j.Status = "unknown"
	j.Error = "Installation exited successfully; tool presence could not be verified."
	for _, t := range tools {
		if t.ID == j.ToolID && t.Path != "" && t.Status != "missing" && t.Status != "error" {
			j.Status = "installed_pending"
			j.Error = ""
			if t.Selectable {
				j.Status = "ready"
			}
			break
		}
	}
	j.UpdatedAt = time.Now().UTC()
	if i.save(j) == nil && j.Status != "unknown" {
		_ = os.Remove(lock)
	}
}
func (i *Installer) Get(id string) (Job, error) { i.mu.Lock(); defer i.mu.Unlock(); return i.get(id) }
func (i *Installer) GetPlan(id string) (Plan, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !validID(id) {
		return Plan{}, errors.New("invalid plan id")
	}
	var p Plan
	err := i.read("plan-"+id+".json", &p)
	return p, err
}

// Reconcile observes the original process identity and presence, never restarts
// the installer. Unverifiable ownership requires operator review and keeps its
// cross-process lock; a discovered binary alone cannot prove the job stopped.
func (i *Installer) Reconcile(ctx context.Context, id string) (Job, error) {
	i.mu.Lock()
	if !validID(id) {
		i.mu.Unlock()
		return Job{}, errors.New("invalid job id")
	}
	reconcileLock := filepath.Join(i.dir, "reconcile-"+id)
	fence, lockErr := os.OpenFile(reconcileLock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if lockErr != nil {
		i.mu.Unlock()
		return Job{}, errors.New("reconciliation already owned or requires operator review")
	}
	_ = fence.Close()
	defer os.Remove(reconcileLock)
	j, err := i.get(id)
	if err != nil {
		i.mu.Unlock()
		return j, err
	}
	if j.Status != "unknown" {
		i.mu.Unlock()
		return j, nil
	}
	if i.cancels[id] != nil {
		i.mu.Unlock()
		return j, errors.New("installation still owned by an active job")
	}
	if j.ProcessID <= 0 || j.ProcessIdentity == "" || j.OwnerIdentity == "" {
		i.mu.Unlock()
		return j, errors.New("operator review required: original process identity was not durably established")
	}
	owner, ownerErr := processIdentity(j.OwnerID)
	if !j.ProcessFinished && ownerErr == nil && owner == j.OwnerIdentity {
		i.mu.Unlock()
		return j, errors.New("original installer service remains alive; observe it before reconciliation")
	}
	if !j.ProcessFinished && ownerErr != nil && !errors.Is(ownerErr, os.ErrNotExist) {
		i.mu.Unlock()
		return j, errors.New("operator review required: original service liveness unverifiable")
	}
	current, identityErr := processIdentity(j.ProcessID)
	if identityErr == nil && current == j.ProcessIdentity {
		i.mu.Unlock()
		return j, errors.New("original installer process remains alive")
	}
	if identityErr != nil && !errors.Is(identityErr, os.ErrNotExist) {
		i.mu.Unlock()
		return j, errors.New("operator review required: original installer liveness unverifiable")
	}
	if !processTreeAbsent(j.ProcessID) {
		i.mu.Unlock()
		return j, errors.New("operator review required: original process tree absence unproven")
	}
	i.mu.Unlock()
	tools := i.discover(ctx)
	i.mu.Lock()
	defer i.mu.Unlock()
	if ctx.Err() != nil {
		return j, ctx.Err()
	}
	j.Status = "failed"
	j.Error = "Original installer stopped; discovery did not confirm the tool. A new plan may be reviewed."
	found := false
	for _, t := range tools {
		if t.ID != j.ToolID {
			continue
		}
		found = true
		if t.Path != "" && t.Status != "error" && t.Status != "missing" {
			j.Status = "installed_pending"
			j.Error = ""
			if t.Selectable {
				j.Status = "ready"
			}
		} else if t.Status != "missing" {
			return j, errors.New("tool discovery inconclusive; operator review required")
		}
		break
	}
	if !found {
		return j, errors.New("tool discovery unavailable; operator review required")
	}
	j.UpdatedAt = time.Now().UTC()
	if err := i.save(j); err != nil {
		return j, err
	}
	if err := os.Remove(filepath.Join(i.dir, "lock-"+j.HostID+"-"+j.ToolID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return j, err
	}
	return j, nil
}
func (i *Installer) get(id string) (Job, error) {
	if !validID(id) {
		return Job{}, errors.New("invalid job id")
	}
	var stored storedJob
	if err := i.read("job-"+id+".json", &stored); err != nil {
		return Job{}, err
	}
	j := stored.Job
	j.IdempotencyKey = stored.Key
	j.PlanHash = stored.Hash
	if (j.Status == "installing" || j.Status == "verifying" || j.Status == "cancel_requested") && i.cancels[id] == nil {
		j.Status = "unknown"
		j.Error = "Process ownership was lost; reconcile installation effects before retrying."
	}
	return j, nil
}
func (i *Installer) Cancel(id string) (Job, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	j, err := i.get(id)
	if err != nil {
		return j, err
	}
	cancel := i.cancels[id]
	if cancel == nil {
		return j, errors.New("no owned running process; reconcile instead of killing an unrelated process")
	}
	j.Status = "cancel_requested"
	j.UpdatedAt = time.Now().UTC()
	if err := i.save(j); err != nil {
		return j, err
	}
	cancel()
	return j, nil
}
func (i *Installer) jobs() ([]Job, error) {
	entries, err := os.ReadDir(i.dir)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "job-") && strings.HasSuffix(e.Name(), ".json") {
			id := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "job-"), ".json")
			j, err := i.get(id)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}
func (i *Installer) save(j Job) error {
	return i.write("job-"+j.ID+".json", storedJob{Job: j, Key: j.IdempotencyKey, Hash: j.PlanHash})
}
func (i *Installer) read(name string, v any) error {
	data, err := os.ReadFile(filepath.Join(i.dir, name))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("invalid installation record: %w", err)
	}
	return nil
}
func (i *Installer) write(name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(i.dir, "record-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), filepath.Join(i.dir, name))
}
