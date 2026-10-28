package discovery

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// Format writes aligned, color-free rows usable by doctor and /agents.
func Format(w io.Writer, results []Result) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, r := range results {
		auth := string(r.Auth)
		if r.Auth == "" || r.Auth == AuthUnknown {
			auth = r.Note
			if auth == "" {
				auth = authNote
			}
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sanitize(r.Name, 200), sanitize(string(r.Status), 200),
			sanitize(r.Version, versionLimit), sanitize(r.Path, 4096),
			sanitize(auth, 200), sanitize(r.Err, 200)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// FormatJSON writes the structured result array for scripts. Probe output is
// represented only by bounded, sanitized fields in Result.
func FormatJSON(w io.Writer, results []Result) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(results)
}
