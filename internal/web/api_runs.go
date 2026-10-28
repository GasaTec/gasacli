package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/storage"
)

func runsAPI(home string, w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/api/runs" && r.Method == http.MethodGet {
		core, closeStore, err := openRunAPI(home)
		if err != nil {
			http.Error(w, "run data unavailable", http.StatusServiceUnavailable)
			return true
		}
		defer closeStore()
		runs, err := core.ListRuns(r.Context(), 100)
		if err != nil {
			http.Error(w, "run data unavailable", http.StatusInternalServerError)
			return true
		}
		jsonResponse(w, runs)
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/api/runs/") || r.Method != http.MethodGet {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/runs/"), "/")
	if len(parts) < 1 || parts[0] == "" || strings.Contains(parts[0], "\\") {
		http.NotFound(w, r)
		return true
	}
	core, closeStore, err := openRunAPI(home)
	if err != nil {
		http.Error(w, "run data unavailable", http.StatusServiceUnavailable)
		return true
	}
	defer closeStore()
	if len(parts) == 2 && parts[1] == "events" {
		serveRunEvents(core, w, r, parts[0])
		return true
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return true
	}
	run, err := core.GetRun(r.Context(), parts[0])
	if err != nil {
		http.NotFound(w, r)
		return true
	}
	tasks, err := core.Tasks(r.Context(), run.ID)
	if err != nil {
		http.Error(w, "run data unavailable", http.StatusInternalServerError)
		return true
	}
	jsonResponse(w, map[string]any{"run": run, "tasks": tasks})
	return true
}

func openRunAPI(home string) (*engine.Engine, func(), error) {
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		return nil, func() {}, err
	}
	core, err := engine.New(store)
	if err != nil {
		_ = store.Close()
		return nil, func() {}, err
	}
	return core, func() { _ = store.Close() }, nil
}

func serveRunEvents(core *engine.Engine, w http.ResponseWriter, r *http.Request, runID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unavailable", http.StatusInternalServerError)
		return
	}
	lastID := int64(0)
	if value := r.Header.Get("Last-Event-ID"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed >= 0 {
			lastID = parsed
		}
	}
	if value := r.URL.Query().Get("after"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed >= 0 {
			lastID = parsed
		} else {
			http.Error(w, "invalid event cursor", http.StatusBadRequest)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, err := core.Events(r.Context(), runID, lastID, 200)
		if err != nil {
			_, _ = fmt.Fprint(w, "event: error\ndata: {}\n\n")
			flusher.Flush()
			return
		}
		for _, event := range events {
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, data); err != nil {
				return
			}
			lastID = event.ID
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
