// Package memory exposes cited snippets for future engine context injection.
package memory

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/vault"
)

type Snippet = vault.Snippet
type Retriever interface {
	Search(context.Context, string, int) ([]vault.Snippet, error)
}

// Retrieve returns cited snippets within an approximate token budget. It performs local retrieval only.
func Retrieve(ctx context.Context, query string, budgetTokens int) []Snippet {
	if ctx == nil {
		ctx = context.Background()
	}
	home, err := config.Home()
	if err != nil {
		return nil
	}
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		return nil
	}
	defer store.Close()
	retriever, err := vault.New(store)
	if err != nil {
		return nil
	}
	return RetrieveFrom(ctx, retriever, query, budgetTokens)
}

// RetrieveFrom allows engine callers and tests to provide an existing local retriever.
func RetrieveFrom(ctx context.Context, retriever Retriever, query string, budgetTokens int) []Snippet {
	if retriever == nil || budgetTokens <= 0 {
		return nil
	}
	limit := budgetTokens / 80
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	found, err := retriever.Search(ctx, query, limit)
	if err != nil {
		return nil
	}
	out := make([]Snippet, 0, len(found))
	used := 0
	for _, s := range found {
		cost := max(1, (len(strings.Fields(s.Text))+len(strings.Fields(s.Heading)))/3)
		if used+cost > budgetTokens {
			break
		}
		out = append(out, s)
		used += cost
	}
	return out
}
