package cli

import (
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/lfreixial/plane-cli/internal/api"
	"github.com/spf13/cobra"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (a *app) findIssue(cmd *cobra.Command, ref string) (api.Object, string, error) {
	if err := a.connect(); err != nil {
		return nil, "", err
	}
	var o api.Object
	var p string
	var err error
	if uuid.MatchString(ref) {
		p, err = a.projectID(cmd)
		if err != nil {
			return nil, "", err
		}
		o, err = a.get(cmd, a.pp(p)+"work-items/"+ref+"/")
	} else if m := issueKey.FindStringSubmatch(ref); m != nil && segment.MatchString(m[1]) {
		o, err = a.get(cmd, a.ws()+"work-items/"+strings.ToUpper(ref)+"/")
		if err == nil {
			p = o.ID("project")
			if p == "" {
				p, err = a.resolve(cmd, a.ws()+"projects/", m[1], "project")
			}
			o["key"] = strings.ToUpper(ref)
		}
	} else {
		return nil, "", fmt.Errorf("work item must be a key such as ENG-42 or a UUID")
	}
	if err != nil {
		return nil, "", err
	}
	if !uuid.MatchString(o.String("id")) || !uuid.MatchString(p) {
		return nil, "", fmt.Errorf("API returned a work item without a valid ID or project")
	}
	return o, p, nil
}

func (a *app) issueCmd() *cobra.Command {
	r := &cobra.Command{Use: "issue", Aliases: []string{"issues", "work-item", "work-items"}, Short: "Create, browse, and update work items"}
	r.AddCommand(a.issueList(), a.issueWrite(false), a.issueWrite(true), a.commentCmd())
	r.AddCommand(&cobra.Command{Use: "view ISSUE", Short: "View a work item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		o, _, err := a.findIssue(cmd, args[0])
		if err != nil {
			return err
		}
		return a.detail(o)
	}})
	r.AddCommand(&cobra.Command{Use: "delete ISSUE", Short: "Delete a work item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		o, p, err := a.findIssue(cmd, args[0])
		if err != nil {
			return err
		}
		if err := a.confirm("Delete " + Safe(args[0]) + " (" + Safe(o.String("name")) + ")"); err != nil {
			return err
		}
		return a.result(cmd, "DELETE", a.pp(p)+"work-items/"+o.String("id")+"/", nil)
	}})
	r.AddCommand(&cobra.Command{Use: "move ISSUE STATE", Short: "Move a work item to a state (name or UUID)", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		o, p, err := a.findIssue(cmd, args[0])
		if err != nil {
			return err
		}
		state, err := a.resolve(cmd, a.pp(p)+"states/", args[1], "state")
		if err != nil {
			return err
		}
		return a.result(cmd, "PATCH", a.pp(p)+"work-items/"+o.String("id")+"/", map[string]any{"state": state})
	}})
	r.AddCommand(&cobra.Command{Use: "assign ISSUE MEMBER...", Short: "Replace assignees with member names, emails, or UUIDs; use 'none' to clear", Args: cobra.MinimumNArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		o, p, err := a.findIssue(cmd, args[0])
		if err != nil {
			return err
		}
		ids, err := a.resolveMany(cmd, a.pp(p)+"project-members/", args[1:], "member")
		if err != nil {
			return err
		}
		return a.result(cmd, "PATCH", a.pp(p)+"work-items/"+o.String("id")+"/", map[string]any{"assignees": ids})
	}})
	var printOnly bool
	open := &cobra.Command{Use: "open ISSUE", Aliases: []string{"browse"}, Short: "Open a work item in your web browser", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		o, p, err := a.findIssue(cmd, args[0])
		if err != nil {
			return err
		}
		base := a.cfg.WebURL
		if base == "" {
			base = strings.TrimSuffix(strings.TrimRight(a.cfg.BaseURL, "/"), "/api/v1")
			if base == "https://api.plane.so" {
				base = "https://app.plane.so"
			}
		}
		if _, err := api.ValidateURL(base); err != nil {
			return err
		}
		u := strings.TrimRight(base, "/") + "/" + a.cfg.Workspace + "/projects/" + p + "/issues/" + o.String("id")
		if a.json {
			return a.writeJSON(map[string]string{"url": u})
		}
		if printOnly {
			_, err := fmt.Fprintln(a.out, u)
			return err
		}
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.CommandContext(cmd.Context(), "open", u)
		case "windows":
			command = exec.CommandContext(cmd.Context(), "rundll32", "url.dll,FileProtocolHandler", u)
		default:
			command = exec.CommandContext(cmd.Context(), "xdg-open", u)
		}
		if err := command.Run(); err != nil {
			return fmt.Errorf("open browser: %w; URL: %s", err, u)
		}
		return nil
	}}
	open.Flags().BoolVar(&printOnly, "print", false, "Print the URL without launching a browser")
	r.AddCommand(open)
	return r
}

func (a *app) issueList() *cobra.Command {
	var limit int
	var state, assignee, priority, search, order string
	c := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "Browse work items (interactive in a terminal; table when piped)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if limit < 0 {
			return fmt.Errorf("limit cannot be negative")
		}
		if priority != "" && !oneOf(priority, "none", "urgent", "high", "medium", "low") {
			return fmt.Errorf("invalid priority %q", priority)
		}
		p, err := a.projectID(cmd)
		if err != nil {
			return err
		}
		stateID, assigneeID := "", ""
		if state != "" {
			stateID, err = a.resolve(cmd, a.pp(p)+"states/", state, "state")
			if err != nil {
				return err
			}
		}
		if assignee != "" {
			assigneeID, err = a.resolve(cmd, a.pp(p)+"project-members/", assignee, "member")
			if err != nil {
				return err
			}
		}
		q := query()
		q.Set("expand", "state,assignees,labels,project")
		q.Set("order_by", order)
		fetchLimit := limit
		if state != "" || assignee != "" || priority != "" || search != "" {
			fetchLimit = 0
		}
		items, err := a.client.List(cmd.Context(), a.pp(p)+"work-items/", q, fetchLimit)
		if err != nil {
			return err
		}
		filtered := []api.Object{}
		for _, o := range items {
			if stateID != "" && o.ID("state") != stateID {
				continue
			}
			if assigneeID != "" && !containsID(o["assignees"], assigneeID) {
				continue
			}
			if priority != "" && o.String("priority") != priority {
				continue
			}
			if search != "" && !strings.Contains(strings.ToLower(o.String("name")+" "+o.String("description_stripped")), strings.ToLower(search)) {
				continue
			}
			key := o.String("project_identifier")
			if project, ok := o["project"].(map[string]any); ok {
				key, _ = project["identifier"].(string)
			}
			if key != "" {
				o["key"] = key + "-" + o.String("sequence_id")
			} else {
				o["key"] = o.String("sequence_id")
			}
			filtered = append(filtered, o)
			if limit > 0 && len(filtered) == limit {
				break
			}
		}
		if a.interactive() {
			return a.browse(cmd, filtered)
		}
		return a.table(filtered, []string{"key", "name", "state", "priority", "id"})
	}}
	f := c.Flags()
	f.IntVar(&limit, "limit", 100, "Maximum matching work items (0 for all)")
	f.StringVarP(&state, "state", "s", "", "Filter by state name or UUID")
	f.StringVarP(&assignee, "assignee", "a", "", "Filter by member name, email, or UUID")
	f.StringVar(&priority, "priority", "", "Filter by priority")
	f.StringVar(&search, "search", "", "Case-insensitive text search in title and description")
	f.StringVar(&order, "order-by", "-created_at", "API sort field; prefix '-' for descending")
	return c
}

func containsID(v any, id string) bool {
	items, _ := v.([]any)
	for _, x := range items {
		if s, ok := x.(string); ok && s == id {
			return true
		}
		if m, ok := x.(map[string]any); ok && m["id"] == id {
			return true
		}
	}
	return false
}

func (a *app) resolveMany(cmd *cobra.Command, path string, refs []string, kind string) ([]string, error) {
	ids := []string{}
	if len(refs) == 1 && refs[0] == "none" {
		return ids, nil
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		id, err := a.resolve(cmd, path, ref, kind)
		if err != nil {
			return nil, err
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func textHTML(s string) string {
	return "<p>" + strings.ReplaceAll(html.EscapeString(s), "\n", "<br>") + "</p>"
}

func (a *app) body(text, file string) (string, error) {
	if file == "" {
		return text, nil
	}
	var b []byte
	var err error
	if file == "-" {
		b, err = io.ReadAll(io.LimitReader(a.in, 4<<20+1))
	} else {
		f, e := os.Open(file)
		if e != nil {
			return "", e
		}
		defer f.Close()
		b, err = io.ReadAll(io.LimitReader(f, 4<<20+1))
	}
	if err != nil {
		return "", err
	}
	if len(b) > 4<<20 {
		return "", fmt.Errorf("body exceeds 4 MiB")
	}
	return string(b), nil
}

func (a *app) issueWrite(edit bool) *cobra.Command {
	use, short, check := "create", "Create a work item", cobra.NoArgs
	if edit {
		use, short, check = "edit ISSUE", "Update a work item", cobra.ExactArgs(1)
	}
	var title, body, file, state, priority, start, target string
	var assignees, labels []string
	c := &cobra.Command{Use: use, Short: short, Args: check, RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		if !edit && title == "" {
			title, err = a.prompt("Title", "")
			if err != nil {
				return err
			}
		}
		if (!edit || cmd.Flags().Changed("title")) && strings.TrimSpace(title) == "" {
			return fmt.Errorf("title cannot be empty")
		}
		if cmd.Flags().Changed("priority") && !oneOf(priority, "none", "urgent", "high", "medium", "low") {
			return fmt.Errorf("priority must be none, urgent, high, medium, or low")
		}
		payload := map[string]any{}
		if !edit || cmd.Flags().Changed("title") {
			payload["name"] = title
		}
		if cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file") {
			b, err := a.body(body, file)
			if err != nil {
				return err
			}
			payload["description_html"] = textHTML(b)
		}
		if cmd.Flags().Changed("priority") {
			payload["priority"] = priority
		}
		for flag, v := range map[string]string{"start": start, "target": target} {
			if cmd.Flags().Changed(flag) {
				field := flag + "_date"
				if v == "none" {
					payload[field] = nil
				} else {
					if _, err := time.Parse("2006-01-02", v); err != nil {
						return fmt.Errorf("--%s must be YYYY-MM-DD or none", flag)
					}
					payload[field] = v
				}
			}
		}
		if start != "" && target != "" && start != "none" && target != "none" && target < start {
			return fmt.Errorf("target date must be on or after start date")
		}
		if edit && len(payload) == 0 && !cmd.Flags().Changed("state") && !cmd.Flags().Changed("assignee") && !cmd.Flags().Changed("label") {
			return fmt.Errorf("supply at least one field to edit")
		}
		var p, path string
		method := "POST"
		if edit {
			o, project, err := a.findIssue(cmd, args[0])
			if err != nil {
				return err
			}
			p = project
			path = a.pp(p) + "work-items/" + o.String("id") + "/"
			method = "PATCH"
		} else {
			p, err = a.projectID(cmd)
			if err != nil {
				return err
			}
			path = a.pp(p) + "work-items/"
		}
		if cmd.Flags().Changed("state") {
			id, err := a.resolve(cmd, a.pp(p)+"states/", state, "state")
			if err != nil {
				return err
			}
			payload["state"] = id
		}
		for flag, refs := range map[string][]string{"assignee": assignees, "label": labels} {
			if cmd.Flags().Changed(flag) {
				resource, field := "project-members/", "assignees"
				if flag == "label" {
					resource, field = "labels/", "labels"
				}
				ids, err := a.resolveMany(cmd, a.pp(p)+resource, refs, flag)
				if err != nil {
					return err
				}
				payload[field] = ids
			}
		}
		return a.result(cmd, method, path, payload)
	}}
	f := c.Flags()
	f.StringVarP(&title, "title", "t", "", "Work item title")
	f.StringVarP(&body, "body", "b", "", "Plain-text description (HTML escaped)")
	f.StringVar(&file, "body-file", "", "Read description from a file, or '-' for stdin")
	f.StringVarP(&state, "state", "s", "", "State name or UUID")
	f.StringVar(&priority, "priority", "", "none, urgent, high, medium, low")
	f.StringSliceVarP(&assignees, "assignee", "a", nil, "Members by name/email/UUID, comma-separated; 'none' clears")
	f.StringSliceVarP(&labels, "label", "l", nil, "Label names/UUIDs, comma-separated; 'none' clears")
	f.StringVar(&start, "start", "", "Start date (YYYY-MM-DD); 'none' clears")
	f.StringVar(&target, "target", "", "Target date (YYYY-MM-DD); 'none' clears")
	c.MarkFlagsMutuallyExclusive("body", "body-file")
	return c
}

func (a *app) commentCmd() *cobra.Command {
	r := &cobra.Command{Use: "comment", Short: "Read and add work item comments"}
	r.AddCommand(&cobra.Command{Use: "list ISSUE", Short: "List comments", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		o, p, err := a.findIssue(cmd, args[0])
		if err != nil {
			return err
		}
		items, err := a.client.List(cmd.Context(), a.pp(p)+"work-items/"+o.String("id")+"/comments/", nil, 0)
		if err != nil {
			return err
		}
		return a.table(items, []string{"id", "actor", "comment_html", "created_at"})
	}})
	var body, file string
	c := &cobra.Command{Use: "add ISSUE [TEXT]", Short: "Add a plain-text comment", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 2 {
			if cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file") {
				return fmt.Errorf("use TEXT or a body flag, not both")
			}
			body = args[1]
		}
		b, err := a.body(body, file)
		if err != nil {
			return err
		}
		if b == "" {
			b, err = a.prompt("Comment", "")
			if err != nil {
				return err
			}
		}
		if strings.TrimSpace(b) == "" {
			return fmt.Errorf("comment cannot be empty")
		}
		o, p, err := a.findIssue(cmd, args[0])
		if err != nil {
			return err
		}
		return a.result(cmd, "POST", a.pp(p)+"work-items/"+o.String("id")+"/comments/", map[string]any{"comment_html": textHTML(b)})
	}}
	c.Flags().StringVarP(&body, "body", "b", "", "Comment text")
	c.Flags().StringVar(&file, "body-file", "", "Read comment from file or '-' for stdin")
	c.MarkFlagsMutuallyExclusive("body", "body-file")
	r.AddCommand(c)
	return r
}
