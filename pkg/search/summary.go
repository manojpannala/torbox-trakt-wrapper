package search

import (
	"fmt"
	"log/slog"
	"strings"
)

type Tally struct {
	Received int // rows from every indexer, before the title filter
	Merged   int // rows after Merge
	Dropped  map[DropReason]int
	Late     int // rows that went to Late results
}

// Attrs is the summary line's fields, for logger.Debug("search summary",
// t.Attrs(sections)...). It never includes a release name.
func (t Tally) Attrs(sections []Section) []any {
	var groups []string
	for _, s := range sections {
		if s.Late {
			continue
		}
		groups = append(groups, fmt.Sprintf("%s %d/%d", s.Group.Header(), s.Shown, len(s.Rows)))
	}
	attrs := []any{
		slog.Int("received", t.Received),
		slog.Int("merged", t.Merged),
		slog.String("groups", strings.Join(groups, " ")),
	}
	for r := DropTitle; r <= DropUnresolved; r++ {
		attrs = append(attrs, slog.Int("dropped_"+r.String(), t.Dropped[r]))
	}
	attrs = append(attrs, slog.Int("late", t.Late))
	return attrs
}
