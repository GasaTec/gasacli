package discovery

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// Format writes aligned, color-free rows usable by doctor and /agents.
func Format(w io.Writer, results []Result) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, r := range results {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sanitize(r.Name, 200), sanitize(string(r.Status), 200),
			sanitize(r.Version, versionLimit), sanitize(r.Path, 4096),
			authNote, sanitize(r.Err, 200)); err != nil {
			return err
		}
	}
	return tw.Flush()
}
