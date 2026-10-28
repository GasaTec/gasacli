package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GasaTec/gasacli/internal/brain"
)

func runBrain(ctx context.Context, args []string, out io.Writer) error {
	result, err := brain.Command(ctx, args)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, result)
	return err
}
