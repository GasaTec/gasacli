package github

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

func ParseIssueFilters(args []string) (IssueFilters, error) {
	filters := IssueFilters{}
	positionalState := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalState, args = args[0], args[1:]
	}
	var labels string
	set := flag.NewFlagSet("github issues", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&filters.State, "state", "", "issue state")
	set.StringVar(&filters.Assignee, "assignee", "", "assignee login")
	set.StringVar(&labels, "label", "", "comma-separated labels")
	set.StringVar(&filters.Search, "search", "", "GitHub issue search")
	set.IntVar(&filters.Limit, "limit", 30, "maximum issue count")
	if err := set.Parse(args); err != nil {
		return IssueFilters{}, err
	}
	if len(set.Args()) > 0 {
		return IssueFilters{}, fmt.Errorf("usage: github issues [state] [--assignee USER] [--label LABELS] [--search QUERY] [--limit N]")
	}
	if filters.State == "" {
		filters.State = positionalState
	}
	if strings.TrimSpace(labels) != "" {
		for _, label := range strings.Split(labels, ",") {
			if label = strings.TrimSpace(label); label != "" {
				filters.Labels = append(filters.Labels, label)
			}
		}
	}
	return filters, nil
}
