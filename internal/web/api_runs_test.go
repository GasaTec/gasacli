package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/storage"
)

func TestRunsAPIListsAndReturnsRun(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	run, err := core.CreateRun(context.Background(), t.TempDir(), "sample goal")
	if err != nil {
		t.Fatal(err)
	}
	if err = core.AddTasks(context.Background(), run.ID, []engine.Task{{ID: "one", Name: "task"}}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	list := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	if !runsAPI(home, list, req) {
		t.Fatal("runs route not handled")
	}
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d", list.Code)
	}
	var runs []engine.Run
	if err = json.Unmarshal(list.Body.Bytes(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %#v", runs)
	}
	detail := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID, nil)
	if !runsAPI(home, detail, req) {
		t.Fatal("detail route not handled")
	}
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "sample goal") {
		t.Fatalf("detail response: %d %s", detail.Code, detail.Body.String())
	}
}

func TestRunsAPIEventStreamResumesFromCursor(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	run, err := core.CreateRun(context.Background(), t.TempDir(), "events")
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID+"/events", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { runsAPI(home, recorder, req); close(done) }()
	for i := 0; i < 50 && !strings.Contains(recorder.Body.String(), "run.created"); i++ {
		select {
		case <-done:
			t.Fatal("event stream ended early")
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if !strings.Contains(recorder.Body.String(), "event: run.created") {
		t.Fatalf("stream body = %q", recorder.Body.String())
	}
}
