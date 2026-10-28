package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	"github.com/GasaTec/gasacli/internal/brain"
	"github.com/GasaTec/gasacli/internal/i18n"
)

func runBrain(ctx context.Context, args []string, out io.Writer) error {
	result, err := brain.Command(ctx, args)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, writeErr := fmt.Fprintln(out, i18n.T("brain.not_connected"))
			return writeErr
		}
		return err
	}
	_, err = fmt.Fprintln(out, result)
	return err
}
