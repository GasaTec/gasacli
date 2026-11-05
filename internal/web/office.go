package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/integrations/host"
	"github.com/GasaTec/gasacli/internal/office"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/trust"
)

// officeAPI runs inside the existing authentication and Host/Origin boundary.
// Opening a page or saving a character never starts a worker.
type officeAPI struct {
	home      string
	projects  func() snapshot
	toolsMu   sync.Mutex
	tools     []host.Tool
	checkedAt time.Time
	installer *host.Installer
	runtimeMu sync.Mutex
	runtimes  map[string]*officeRuntimeService
}

type officeMutation struct {
	ProjectID      string `json:"projectId"`
	IdempotencyKey string `json:"idempotencyKey"`
	Revision       int    `json:"revision"`
	office.AgentInput
}

func (a *officeAPI) serve(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/office/") {
		return false
	}
	if a.home == "" {
		officeFailure(w, http.StatusServiceUnavailable, "office_unavailable", "O armazenamento do escritório não está disponível.")
		return true
	}
	if a.serveRuntime(w, r) {
		return true
	}
	if a.serveTools(w, r) {
		return true
	}
	if r.URL.Path == "/api/office/capabilities" && r.Method == http.MethodGet {
		jsonResponse(w, map[string]any{"persistentOffice": true, "createAgent": true, "teams": true, "messages": true, "coordination": "pending_runtime_binding", "dispatch": false, "screenControl": false, "voice": "browser_optional", "plugins": "host_read_only", "reason": "A execução supervisionada exige vínculo autorizado com o coordenador do Orca."})
		return true
	}
	projectID := r.URL.Query().Get("projectId")
	var raw json.RawMessage
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		if err := dec.Decode(&raw); err != nil || dec.Decode(new(any)) != io.EOF {
			officeFailure(w, 400, "invalid_request", "Envie um objeto JSON válido de até 64 KiB.")
			return true
		}
		var scope struct {
			ProjectID string `json:"projectId"`
		}
		if json.Unmarshal(raw, &scope) != nil || len(raw) == 0 || raw[0] != '{' {
			officeFailure(w, 400, "invalid_request", "Objeto JSON obrigatório.")
			return true
		}
		if projectID != "" && scope.ProjectID != "" && projectID != scope.ProjectID {
			officeFailure(w, 400, "project_mismatch", "O projeto da operação não corresponde à tela atual.")
			return true
		}
		if scope.ProjectID != "" {
			projectID = scope.ProjectID
		}
	}
	project, projectErr := a.project(projectID)
	if projectErr != nil {
		officeFailure(w, 403, "project_untrusted", "Selecione um projeto confiável antes de usar o escritório.")
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		unlock := a.lockOfficeMutation(project.Path)
		defer unlock()
	}
	store, err := storage.Open(filepath.Join(a.home, "gasacli.db"))
	if err != nil {
		officeFailure(w, 503, "storage_unavailable", "Não foi possível abrir o armazenamento do escritório.")
		return true
	}
	defer store.Close()
	core := office.New(store)
	path := strings.TrimPrefix(r.URL.Path, "/api/office/")
	switch {
	case path == "snapshot" && r.Method == http.MethodGet:
		value, e := core.Snapshot(r.Context(), projectID)
		officeResult(w, value, e)
	case path == "events" && r.Method == http.MethodGet:
		a.events(core, projectID, w, r)
	case path == "agents" && r.Method == http.MethodPost:
		var in officeMutation
		if !officeDecode(raw, &in, w) {
			return true
		}
		replayed, found, replayErr := core.ReplayAgent(r.Context(), projectID, in.IdempotencyKey, in.AgentInput)
		if replayErr != nil || found {
			officeResult(w, replayed, replayErr)
			return true
		}
		if !a.validateChoices(w, r, in.AgentInput, nil) {
			return true
		}
		value, e := core.CreateAgent(r.Context(), projectID, in.IdempotencyKey, in.AgentInput)
		officeResult(w, value, e)
	case strings.HasPrefix(path, "agents/") && r.Method == http.MethodPut:
		id := strings.TrimPrefix(path, "agents/")
		if !officeID(id) {
			officeFailure(w, 404, "not_found", "Agente não encontrado.")
			return true
		}
		var in officeMutation
		if !officeDecode(raw, &in, w) {
			return true
		}
		current, e := core.Snapshot(r.Context(), projectID)
		if e != nil {
			officeResult(w, nil, e)
			return true
		}
		var previous *office.AgentInput
		for _, candidate := range current.Agents {
			if candidate.ID == id {
				saved := candidate.AgentInput
				previous = &saved
				break
			}
		}
		if previous != nil && !sameExecutionConfiguration(*previous, in.AgentInput) {
			busy, checkErr := officeExternalWork(r.Context(), store, projectID)
			if checkErr != nil { officeResult(w, nil, checkErr); return true }
			if busy { officeResult(w, nil, office.ErrBusy); return true }
		}
		if !a.validateChoices(w, r, in.AgentInput, previous) {
			return true
		}
		value, e := core.UpdateAgent(r.Context(), projectID, id, in.Revision, in.AgentInput)
		officeResult(w, value, e)
	case (path == "teams" && r.Method == http.MethodPost) || (strings.HasPrefix(path, "teams/") && r.Method == http.MethodPut):
		busy, checkErr := officeExternalWork(r.Context(), store, projectID)
		if checkErr != nil { officeResult(w, nil, checkErr); return true }
		if busy { officeResult(w, nil, office.ErrBusy); return true }
		var in struct {
			ProjectID string `json:"projectId"`
			IdempotencyKey string `json:"idempotencyKey"`
			office.Team
		}
		if !officeDecode(raw, &in, w) {
			return true
		}
		if strings.HasPrefix(path, "teams/") {
			id := strings.TrimPrefix(path, "teams/")
			if !officeID(id) || (in.ID != "" && in.ID != id) {
				officeFailure(w, 400, "invalid_team", "Equipe inválida.")
				return true
			}
			in.ID = id
		}
		var value office.Team
		var e error
		if r.Method==http.MethodPost {
			value,e=core.SaveTeamWithKey(r.Context(),projectID,in.IdempotencyKey,in.Team)
		}else{
			value,e=core.SaveTeam(r.Context(), projectID, in.Team)
		}
		officeResult(w, value, e)
	case path == "settings" && r.Method == http.MethodPut:
		var in struct {
			ProjectID string `json:"projectId"`
			office.Settings
		}
		if !officeDecode(raw, &in, w) {
			return true
		}
		previous, readErr := core.Snapshot(r.Context(), projectID)
		if readErr != nil { officeResult(w, nil, readErr); return true }
		if in.CoordinatorID != previous.Settings.CoordinatorID || in.Autonomy != previous.Settings.Autonomy || (!in.Paused && previous.Settings.Paused) {
			busy, checkErr := officeExternalWork(r.Context(), store, projectID)
			if checkErr != nil { officeResult(w, nil, checkErr); return true }
			if busy { officeResult(w, nil, office.ErrBusy); return true }
		}
		value, e := core.SaveSettings(r.Context(), projectID, in.Settings)
		officeResult(w, value, e)
	case path == "messages" && r.Method == http.MethodPost:
		var in struct {
			ProjectID      string `json:"projectId"`
			IdempotencyKey string `json:"idempotencyKey"`
			office.MessageInput
		}
		if !officeDecode(raw, &in, w) {
			return true
		}
		value, e := core.SubmitMessage(r.Context(), projectID, in.IdempotencyKey, in.MessageInput)
		officeResult(w, value, e)
	case path == "stop" && r.Method == http.MethodPost:
		value, e := core.Stop(r.Context(), projectID)
		officeResult(w, map[string]any{"settings": value, "status": "dispatch_barrier_set", "workersConfirmedStopped": false, "reason": "Novas atribuições pausadas. Workers externos ainda exigem confirmação de parada pelo runtime."}, e)
	default:
		officeFailure(w, 404, "not_found", "Operação do escritório não disponível.")
	}
	return true
}

func (a *officeAPI) project(id string) (projectView, error) {
	if id == "" || a.projects == nil {
		return projectView{}, errors.New("project required")
	}
	p, ok := a.projects().byID[id]
	if !ok || !p.Trusted || p.Path == "" {
		return projectView{}, errors.New("project untrusted")
	}
	valid, err := trust.NewStore(a.home).IsTrusted(p.Path)
	if err != nil || !valid {
		return projectView{}, errors.New("project trust revoked")
	}
	return p, nil
}

func officeDecode(raw []byte, target any, w http.ResponseWriter) bool {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		officeFailure(w, 400, "invalid_request", "Campos inválidos para esta operação.")
		return false
	}
	return true
}

func officeID(id string) bool {
	return id != "" && len(id) <= 160 && !strings.ContainsAny(id, "/\\\x00\r\n")
}

func officeFailure(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "error": message})
}

func officeResult(w http.ResponseWriter, value any, err error) {
	if err == nil {
		jsonResponse(w, value)
		return
	}
	switch {
	case errors.Is(err, office.ErrValidation):
		officeFailure(w, 400, "invalid_input", "Revise os campos e a configuração do escritório.")
	case errors.Is(err, office.ErrConflict):
		officeFailure(w, 409, "conflict", "A configuração mudou ou esta chave já foi usada para outro pedido. Atualize a tela.")
	case errors.Is(err, office.ErrBusy):
		officeFailure(w, 409, "active_work", "Existe trabalho em andamento que precisa ser conciliado antes desta alteração.")
	case errors.Is(err, office.ErrNotFound):
		officeFailure(w, 404, "not_found", "Registro não encontrado neste escritório.")
	default:
		officeFailure(w, 503, "office_unavailable", "Não foi possível concluir a operação com segurança.")
	}
}

func (a *officeAPI) events(core *office.Store, project string, w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		officeFailure(w, 503, "stream_unavailable", "Eventos indisponíveis.")
		return
	}
	var cursor int64
	value := r.URL.Query().Get("after")
	if value == "" {
		value = r.Header.Get("Last-Event-ID")
	}
	if value != "" {
		parsed, e := strconv.ParseInt(value, 10, 64)
		if e != nil || parsed < 0 {
			officeFailure(w, 400, "invalid_cursor", "Cursor de eventos inválido.")
			return
		}
		cursor = parsed
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	controller := http.NewResponseController(w)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := a.project(project); err != nil {
			return
		}
		items, err := core.Events(r.Context(), project, cursor)
		if err != nil {
			return
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		for _, event := range items {
			data, e := json.Marshal(event)
			if e != nil {
				return
			}
			if _, e = fmt.Fprintf(w, "id: %d\nevent: office\ndata: %s\n\n", event.Cursor, data); e != nil {
				return
			}
			cursor = event.Cursor
		}
		if _, err = io.WriteString(w, ": heartbeat\n\n"); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
