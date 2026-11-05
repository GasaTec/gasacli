package web

import (
	"context"
	"reflect"

	"github.com/GasaTec/gasacli/internal/office"
	"github.com/GasaTec/gasacli/internal/storage"
)

// Serialize office mutations with the final handoff preflight and side effect.
// A stop acknowledgement therefore cannot precede a later launch using an old
// plan. Existing in-flight host operations still require an observed stop.
func (a *officeAPI) lockOfficeMutation(path string) func() {
	a.runtimeMu.Lock()
	service := a.runtimes[path]
	if service == nil {
		return a.runtimeMu.Unlock
	}
	a.runtimeMu.Unlock()
	service.mu.Lock()
	return service.mu.Unlock
}

func officeExternalWork(ctx context.Context, store *storage.Store, project string) (bool, error) {
	var count int
	err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM office_runtime_operations WHERE project_id=? AND status IN ('unknown','submitting','accepted')`, project).Scan(&count)
	return count > 0, err
}

func sameExecutionConfiguration(a, b office.AgentInput) bool {
	// Name/avatar are presentation. These fields affect actual ongoing authority.
	return a.Status == b.Status && a.Role == b.Role && a.Instructions == b.Instructions && a.WhenToCall == b.WhenToCall && reflect.DeepEqual(a.Primary, b.Primary) && reflect.DeepEqual(a.Fallbacks, b.Fallbacks) && a.FallbackMode == b.FallbackMode && reflect.DeepEqual(a.AllowedCapabilities, b.AllowedCapabilities) && reflect.DeepEqual(a.TeamIDs, b.TeamIDs)
}
