package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GasaTec/gasacli/internal/skills"
)

func runSkills(ctx context.Context, args []string, out io.Writer) error {
	result, err := skills.Command(ctx, args)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, result)
	return nil
}
