package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/lfreixial/plane-cli/internal/api"
)

func (a *app) table(items []api.Object, columns []string) error {
	if a.json {
		return a.writeJSON(items)
	}
	w := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
	head := make([]string, len(columns))
	for i, c := range columns {
		head[i] = strings.ToUpper(c)
	}
	fmt.Fprintln(w, strings.Join(head, "\t"))
	for _, item := range items {
		row := make([]string, len(columns))
		for i, key := range columns {
			row[i] = Safe(item.String(key))
		}
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

func detailText(o api.Object) string {
	var b strings.Builder
	for _, k := range []string{"key", "id", "name", "priority", "state", "assignees", "labels", "start_date", "target_date", "description_stripped", "description", "comment_stripped", "comment_html"} {
		if v := o.String(k); v != "" {
			fmt.Fprintf(&b, "%s: %s\n", strings.ReplaceAll(strings.ToUpper(k), "_", " "), Safe(v))
		}
	}
	return b.String()
}

func (a *app) detail(o api.Object) error {
	if a.json {
		return a.writeJSON(o)
	}
	// Keep arbitrary resource fields visible without dumping HTML internals.
	keys := make([]string, 0, len(o))
	for k := range o {
		if k != "description_html" && k != "description_binary" && k != "description_json" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	w := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
	for _, k := range keys {
		if s := o.String(k); s != "" {
			fmt.Fprintf(w, "%s:\t%s\n", Safe(strings.ToUpper(k)), Safe(s))
		}
	}
	return w.Flush()
}
