package skills

import (
	"context"
	"errors"
	"strings"

	"github.com/GasaTec/gasacli/internal/storage"
)

func init() {
	if err := storage.RegisterMigration(storage.Migration{Version: 71, Name: "skill_run_usage", SQL: `CREATE TABLE skill_run_usage (run_id TEXT NOT NULL, skill_name TEXT NOT NULL, activated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(run_id,skill_name));`}); err != nil {
		panic(err)
	}
}

// RecordActivation is an idempotent hook for the future execution engine.
func RecordActivation(ctx context.Context, store *storage.Store, runID, skillName string) error {
	if store == nil {
		return errors.New("skills: storage is required")
	}
	runID = strings.TrimSpace(runID)
	skillName = strings.TrimSpace(skillName)
	if runID == "" || !nameRE.MatchString(skillName) {
		return errors.New("skills: run id and valid skill name are required")
	}
	_, err := store.DB().ExecContext(ctx, `INSERT OR IGNORE INTO skill_run_usage(run_id,skill_name) VALUES(?,?)`, runID, skillName)
	return err
}
