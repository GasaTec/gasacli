package web

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/GasaTec/gasacli/internal/storage"
)

func init() {
	if err := storage.RegisterMigration(storage.Migration{Version: 82, Name: "office_install_plan_scope", SQL: `CREATE TABLE office_install_plan_scope (plan_id TEXT PRIMARY KEY, project_id TEXT NOT NULL);`}); err != nil {
		panic(err)
	}
}

// Installed tools belong to the host, but a plan and its consent belong to the
// selected project. A different trusted project is not an authorization token.
func (a *officeAPI) bindInstallPlan(ctx context.Context, plan, project string) error {
	store, err := storage.Open(filepath.Join(a.home, "gasacli.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.DB().ExecContext(ctx, `INSERT INTO office_install_plan_scope(plan_id,project_id) VALUES(?,?)`, plan, project)
	return err
}

func (a *officeAPI) checkInstallScope(ctx context.Context, plan, project string) error {
	store, err := storage.Open(filepath.Join(a.home, "gasacli.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	var owner string
	if err = store.DB().QueryRowContext(ctx, `SELECT project_id FROM office_install_plan_scope WHERE plan_id=?`, plan).Scan(&owner); err != nil {
		return errors.New("installation plan unavailable")
	}
	if owner != project {
		return errors.New("installation belongs to another project")
	}
	return nil
}
