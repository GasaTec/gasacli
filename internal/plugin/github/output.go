package github

import (
	"fmt"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
)

func FormatAuth(status AuthStatus) string {
	if !status.LoggedIn {
		return i18n.T("github.not_authenticated")
	}
	text := i18n.T("github.logged_in", status.Account, strings.Join(status.Scopes, ", "))
	if len(status.Missing) > 0 {
		text += "; " + i18n.T("github.missing_scopes", strings.Join(status.Missing, ", "))
	}
	return text
}

func FormatOptIn(enabled bool) string {
	value := "disabled"
	if enabled {
		value = "enabled"
	}
	if i18n.Language() == "pt-BR" {
		value = "desativado"
		if enabled {
			value = "ativado"
		}
	}
	return i18n.T("github.opt_in", value)
}

func FormatOptInChanged(enabled bool) string {
	if enabled {
		return i18n.T("github.enabled")
	}
	return i18n.T("github.disabled")
}

func FormatStatus(repo string, enabled bool, auth AuthStatus) string {
	return fmt.Sprintf(i18n.T("github.repository", "%s")+"\n%s\n%s", repo, FormatOptIn(enabled), FormatAuth(auth))
}

func FormatIssues(issues []Issue) string {
	if len(issues) == 0 {
		return i18n.T("github.no_issues")
	}
	var out strings.Builder
	for _, issue := range issues {
		fmt.Fprintln(&out, i18n.T("github.issue_row", issue.Number, issue.State, issue.Title, issue.URL))
	}
	return strings.TrimSpace(out.String())
}
