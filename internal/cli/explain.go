package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/filipbodor/pogo/internal/history"
)

func explain(out io.Writer, matches []history.Match) error {
	if len(matches) == 0 {
		return errors.New("no matching directory; visit it first or use pogo add PATH")
	}
	if _, err := fmt.Fprintf(out, "Winner: %q\n", matches[0].Path); err != nil {
		return err
	}
	if matches[0].PinName != "" {
		if _, err := fmt.Fprintf(out, "Reason: exact pin %q overrides learned ranking.\n", matches[0].PinName); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(out, "Reason: highest learned score; equal scores are ordered by path."); err != nil {
			return err
		}
	}
	for i, match := range matches {
		last := "never recorded"
		if !match.LastVisited.IsZero() {
			last = match.LastVisited.UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(out, "\n%d. %q\n   visits: %d; last visit: %s; age: %.2f days\n   frequency %.4f x recency %.4f x basename %.0f = score %.4f\n",
			i+1, match.Path, match.Visits, last, match.AgeDays,
			match.Frequency, match.RecencyFactor, match.BasenameBoost, match.Score); err != nil {
			return err
		}
	}
	return nil
}
