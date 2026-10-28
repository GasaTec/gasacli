package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/i18n"
)

func configureCLILanguage() {
	language := ""
	if home, err := config.Home(); err == nil {
		if state, err := config.LoadState(home); err == nil {
			language = state.Onboarding.Language
		}
	}
	if override := strings.TrimSpace(os.Getenv("GASACLI_LANG")); override != "" {
		language = override
	}
	i18n.Configure(language)
}

func localizedHelp() { fmt.Fprint(os.Stdout, i18n.T("cli.help")) }

func formatDiagnosticOutput(out io.Writer, results []discovery.Result) error {
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, result := range results {
		verdict := result.Status
		switch result.Auth {
		case discovery.AuthReady:
			verdict = discovery.Ready
		case discovery.AuthNeedsLogin:
			verdict = discovery.NeedsLogin
		}
		status := "doctor.status." + string(verdict)
		reason := "doctor.reason." + string(verdict)
		errText := result.Err
		if errText != "" {
			reason = cliDiagnosticField(errText, 200)
		} else if result.Auth == discovery.AuthUnknown && result.Status == discovery.Installed {
			switch result.Note {
			case "no documented non-interactive authentication probe":
				reason = i18n.T("doctor.reason.auth_probe_unsupported")
			case "authentication status could not be verified":
				reason = i18n.T("doctor.reason.auth_probe_unknown")
			}
		} else {
			reason = i18n.T(reason)
		}
		if _, err := fmt.Fprintf(writer, i18n.T("doctor.row")+"\n",
			cliDiagnosticField(result.Name, 200), i18n.T(status),
			cliDiagnosticField(result.Version, 200), cliDiagnosticField(result.Path, 4096),
			reason, cliDiagnosticField(errText, 200)); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func cliDiagnosticField(value string, maxRunes int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(value, "�"))
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}
