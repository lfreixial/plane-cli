package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) resourcePath(cmd *cobra.Command, kind string) (string, error) {
	if err := a.connect(); err != nil {
		return "", err
	}
	if kind == "project" {
		return a.ws() + "projects/", nil
	}
	p, err := a.projectID(cmd)
	if err != nil {
		return "", err
	}
	return a.pp(p) + kind + "s/", nil
}

func (a *app) resourceCmd(kind string) *cobra.Command {
	r := &cobra.Command{Use: kind, Short: "Manage " + kind + "s", Aliases: []string{kind + "s"}}
	var limit int
	l := &cobra.Command{Use: "list", Short: "List " + kind + "s", Aliases: []string{"ls"}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := a.resourcePath(cmd, kind)
		if err != nil {
			return err
		}
		items, err := a.client.List(cmd.Context(), path, nil, limit)
		if err != nil {
			return err
		}
		columns := []string{"id", "name"}
		switch kind {
		case "project":
			columns = []string{"identifier", "name", "id"}
		case "state":
			columns = []string{"name", "group", "id"}
		case "label":
			columns = []string{"name", "color", "id"}
		case "cycle":
			columns = []string{"name", "start_date", "end_date", "id"}
		case "module":
			columns = []string{"name", "status", "id"}
		}
		return a.table(items, columns)
	}}
	l.Flags().IntVar(&limit, "limit", 100, "Maximum results (0 for all)")
	r.AddCommand(l)
	viewUse, viewArgs := "view REF", cobra.ExactArgs(1)
	if kind == "project" {
		viewUse, viewArgs = "view [REF]", cobra.MaximumNArgs(1)
	}
	r.AddCommand(&cobra.Command{Use: viewUse, Short: "View a " + kind, Args: viewArgs, RunE: func(cmd *cobra.Command, args []string) error {
		ref := a.cfg.Project
		if len(args) > 0 {
			ref = args[0]
		}
		if ref == "" {
			return fmt.Errorf("no project selected: run 'plane project use' or 'plane project view KEY'")
		}
		path, err := a.resourcePath(cmd, kind)
		if err != nil {
			return err
		}
		id, err := a.resolve(cmd, path, ref, kind)
		if err != nil {
			return err
		}
		o, err := a.get(cmd, path+id+"/")
		if err != nil {
			return err
		}
		return a.detail(o)
	}})
	for _, edit := range []bool{false, true} {
		r.AddCommand(a.resourceWrite(kind, edit))
	}
	r.AddCommand(&cobra.Command{Use: "delete REF", Short: "Delete a " + kind, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path, err := a.resourcePath(cmd, kind)
		if err != nil {
			return err
		}
		id, err := a.resolve(cmd, path, args[0], kind)
		if err != nil {
			return err
		}
		if err := a.confirm("Delete " + kind + " " + Safe(args[0])); err != nil {
			return err
		}
		return a.result(cmd, "DELETE", path+id+"/", nil)
	}})
	if kind == "project" {
		r.AddCommand(a.projectUseCmd())
	}
	if kind == "cycle" || kind == "module" {
		a.membershipCommands(r, kind)
	}
	return r
}

func (a *app) resourceWrite(kind string, edit bool) *cobra.Command {
	verb, use := "create", "create"
	argsCheck := cobra.NoArgs
	if edit {
		verb, use, argsCheck = "edit", "edit REF", cobra.ExactArgs(1)
	}
	var name, description, identifier, group, color, start, end, status string
	c := &cobra.Command{Use: use, Short: strings.Title(verb) + " a " + kind, Args: argsCheck, RunE: func(cmd *cobra.Command, args []string) error {
		payload := map[string]any{}
		if !edit && name == "" {
			var err error
			name, err = a.prompt("Name", "")
			if err != nil {
				return err
			}
		}
		if (!edit || cmd.Flags().Changed("name")) && strings.TrimSpace(name) == "" {
			return fmt.Errorf("name cannot be empty")
		}
		for flag, field := range map[string]string{"name": "name", "description": "description", "identifier": "identifier", "group": "group", "color": "color", "start": "start_date", "end": "end_date", "target": "target_date", "status": "status"} {
			values := map[string]string{"name": name, "description": description, "identifier": identifier, "group": group, "color": color, "start": start, "end": end, "target": end, "status": status}
			if cmd.Flags().Changed(flag) || (flag == "name" && !edit) {
				payload[field] = values[flag]
			}
		}
		if kind == "project" && !edit {
			if identifier == "" {
				var err error
				identifier, err = a.prompt("Project identifier", "")
				if err != nil {
					return err
				}
			}
			if !segment.MatchString(identifier) {
				return fmt.Errorf("identifier must contain letters, digits, underscores, or hyphens")
			}
			payload["identifier"] = strings.ToUpper(identifier)
		}
		if kind == "state" {
			if !edit && group == "" {
				group = "backlog"
				payload["group"] = group
			}
			if group != "" && !oneOf(group, "backlog", "unstarted", "started", "completed", "cancelled") {
				return fmt.Errorf("invalid state group %q", group)
			}
			if !edit && color == "" {
				payload["color"] = "#60646C"
			}
		}
		if status != "" && !oneOf(status, "backlog", "planned", "in-progress", "paused", "completed", "cancelled") {
			return fmt.Errorf("invalid module status %q", status)
		}
		if color != "" && !hexColor.MatchString(color) {
			return fmt.Errorf("color must be #RRGGBB")
		}
		for _, s := range []string{start, end} {
			if s != "" {
				if _, err := time.Parse("2006-01-02", s); err != nil {
					return fmt.Errorf("dates must use YYYY-MM-DD")
				}
			}
		}
		if start != "" && end != "" && end < start {
			return fmt.Errorf("end date must be on or after start date")
		}
		if len(payload) == 0 {
			return fmt.Errorf("supply at least one field to edit")
		}
		var path string
		if kind == "cycle" && !edit {
			project, err := a.projectID(cmd)
			if err != nil {
				return err
			}
			// Plane's cycle creation validator reads project_id from the body.
			payload["project_id"] = project
			path = a.pp(project) + "cycles/"
		} else {
			var err error
			path, err = a.resourcePath(cmd, kind)
			if err != nil {
				return err
			}
		}
		method := "POST"
		if edit {
			id, err := a.resolve(cmd, path, args[0], kind)
			if err != nil {
				return err
			}
			path += id + "/"
			method = "PATCH"
		}
		return a.result(cmd, method, path, payload)
	}}
	f := c.Flags()
	f.StringVarP(&name, "name", "n", "", "Name")
	f.StringVarP(&description, "description", "d", "", "Description")
	if kind == "project" {
		f.StringVar(&identifier, "identifier", "", "Project key, e.g. ENG")
	}
	if kind == "state" {
		f.StringVar(&group, "group", "", "backlog, unstarted, started, completed, cancelled")
	}
	if kind == "state" || kind == "label" {
		f.StringVar(&color, "color", "", "Hex color (#RRGGBB)")
	}
	if kind == "cycle" || kind == "module" {
		f.StringVar(&start, "start", "", "Start date (YYYY-MM-DD)")
		if kind == "cycle" {
			f.StringVar(&end, "end", "", "End date (YYYY-MM-DD)")
		} else {
			f.StringVar(&end, "target", "", "Target date (YYYY-MM-DD)")
		}
	}
	if kind == "module" {
		f.StringVar(&status, "status", "", "backlog, planned, in-progress, paused, completed, cancelled")
	}
	return c
}

func (a *app) membershipCommands(parent *cobra.Command, kind string) {
	for _, action := range []string{"issues", "add", "remove"} {
		use, short := "issues REF", "List work items in a "+kind
		check := cobra.ExactArgs(1)
		if action == "add" {
			use, short, check = "add REF ISSUE...", "Add work items to a "+kind, cobra.MinimumNArgs(2)
		}
		if action == "remove" {
			use, short, check = "remove REF ISSUE", "Remove a work item from a "+kind, cobra.ExactArgs(2)
		}
		c := &cobra.Command{Use: use, Short: short, Args: check, RunE: func(cmd *cobra.Command, args []string) error {
			p, err := a.projectID(cmd)
			if err != nil {
				return err
			}
			base := a.pp(p) + kind + "s/"
			id, err := a.resolve(cmd, base, args[0], kind)
			if err != nil {
				return err
			}
			path := base + id + "/" + kind + "-issues/"
			if action == "issues" {
				items, err := a.client.List(cmd.Context(), path, nil, 0)
				if err != nil {
					return err
				}
				return a.table(items, []string{"id", "issue", "name"})
			}
			ids := []string{}
			for _, ref := range args[1:] {
				item, project, err := a.findIssue(cmd, ref)
				if err != nil {
					return err
				}
				if project != p {
					return fmt.Errorf("work item %s belongs to a different project", ref)
				}
				ids = append(ids, item.String("id"))
			}
			if action == "remove" {
				return a.result(cmd, "DELETE", path+ids[0]+"/", nil)
			}
			return a.result(cmd, "POST", path, map[string]any{"issues": ids})
		}}
		parent.AddCommand(c)
	}
}

func (a *app) memberCmd() *cobra.Command {
	r := &cobra.Command{Use: "member", Short: "List members", Aliases: []string{"members"}}
	var workspace bool
	c := &cobra.Command{Use: "list", Short: "List project members or all workspace members", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := a.connect(); err != nil {
			return err
		}
		path := a.ws() + "members/"
		if !workspace {
			p, err := a.projectID(cmd)
			if err != nil {
				return err
			}
			path = a.pp(p) + "project-members/"
		}
		items, err := a.client.List(cmd.Context(), path, nil, 0)
		if err != nil {
			return err
		}
		return a.table(items, []string{"id", "display_name", "email"})
	}}
	c.Flags().BoolVar(&workspace, "all", false, "List workspace members instead of project members")
	r.AddCommand(c)
	return r
}

func oneOf(s string, options ...string) bool {
	for _, o := range options {
		if s == o {
			return true
		}
	}
	return false
}
