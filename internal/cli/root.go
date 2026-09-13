package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/lfreixial/plane-cli/internal/api"
	"github.com/lfreixial/plane-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type app struct {
	path, base, web, workspace, project string
	json, plain, yes                    bool
	timeout                             time.Duration
	stored, cfg                         config.Config
	in                                  io.Reader
	out, errOut                         io.Writer
	reader                              *bufio.Reader
	client                              *api.Client
}

func New(version string, in io.Reader, out, errOut io.Writer) *cobra.Command {
	a := &app{in: in, out: out, errOut: errOut, reader: bufio.NewReader(in)}
	root := &cobra.Command{Use: "plane", Short: "Plane from your terminal", Version: version, SilenceUsage: true, SilenceErrors: true}
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if a.path == "" {
			var err error
			a.path, err = config.Path()
			if err != nil {
				return err
			}
		}
		var err error
		a.stored, err = config.Load(a.path)
		if err != nil {
			return err
		}
		a.cfg = a.stored.WithEnv()
		for flag, pair := range map[string][2]*string{"url": {&a.cfg.BaseURL, &a.base}, "web-url": {&a.cfg.WebURL, &a.web}, "workspace": {&a.cfg.Workspace, &a.workspace}, "project": {&a.cfg.Project, &a.project}} {
			if cmd.Flags().Changed(flag) {
				*pair[0] = *pair[1]
			}
		}
		return nil
	}
	f := root.PersistentFlags()
	f.StringVar(&a.path, "config", "", "Config file (PLANE_CONFIG or user config directory)")
	f.StringVar(&a.base, "url", "", "Plane API origin (PLANE_URL)")
	f.StringVar(&a.web, "web-url", "", "Plane web origin (PLANE_WEB_URL)")
	f.StringVarP(&a.workspace, "workspace", "w", "", "Workspace slug (PLANE_WORKSPACE)")
	f.StringVarP(&a.project, "project", "p", "", "Project identifier, name, or UUID (PLANE_PROJECT)")
	f.BoolVar(&a.json, "json", false, "Print JSON for scripting")
	f.BoolVar(&a.plain, "plain", false, "Print a table instead of the interactive browser")
	f.BoolVarP(&a.yes, "yes", "y", false, "Confirm destructive actions")
	f.DurationVar(&a.timeout, "timeout", 30*time.Second, "HTTP request timeout")
	root.AddCommand(a.initCmd(), a.configCmd(), a.issueCmd(), a.resourceCmd("project"), a.resourceCmd("state"), a.resourceCmd("label"), a.resourceCmd("cycle"), a.resourceCmd("module"), a.memberCmd())
	return root
}

func (a *app) connect() error {
	if strings.TrimSpace(a.cfg.Workspace) == "" {
		return fmt.Errorf("missing workspace: run 'plane init' or use --workspace")
	}
	if !segment.MatchString(a.cfg.Workspace) {
		return fmt.Errorf("workspace must be a slug, not a URL")
	}
	if a.client != nil {
		return nil
	}
	c, err := api.New(a.cfg.BaseURL, a.cfg.APIKey, a.timeout)
	a.client = c
	return err
}

var segment = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var issueKey = regexp.MustCompile(`^(.+)-([0-9]+)$`)

func (a *app) ws() string               { return "workspaces/" + a.cfg.Workspace + "/" }
func (a *app) pp(project string) string { return a.ws() + "projects/" + project + "/" }

func (a *app) projectID(cmd *cobra.Command) (string, error) {
	if err := a.connect(); err != nil {
		return "", err
	}
	ref := a.cfg.Project
	if ref == "" {
		return "", fmt.Errorf("select a project with 'plane project use KEY' or --project")
	}
	return a.resolve(cmd, a.ws()+"projects/", ref, "project")
}

func (a *app) resolve(cmd *cobra.Command, path, ref, kind string) (string, error) {
	if uuid.MatchString(ref) {
		return ref, nil
	}
	items, err := a.client.List(cmd.Context(), path, nil, 0)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, item := range items {
		// Some member APIs wrap the user in a membership record.
		if nested, ok := item["member"].(map[string]any); ok {
			item = api.Object(nested)
		}
		for _, key := range []string{"id", "identifier", "name", "email", "display_name"} {
			if s := item.String(key); s != "" && strings.EqualFold(s, ref) {
				matches = append(matches, item.String("id"))
				break
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous %s %q; use a UUID", kind, ref)
	}
	return "", fmt.Errorf("%s %q not found", kind, ref)
}

func (a *app) get(cmd *cobra.Command, path string) (api.Object, error) {
	var o api.Object
	err := a.client.Do(cmd.Context(), "GET", path, nil, nil, &o)
	return o, err
}

func (a *app) writeJSON(v any) error {
	e := json.NewEncoder(a.out)
	e.SetIndent("", "  ")
	return e.Encode(v)
}

func (a *app) result(cmd *cobra.Command, method, path string, payload any) error {
	var out any
	if err := a.client.Do(cmd.Context(), method, path, nil, payload, &out); err != nil {
		return err
	}
	if a.json {
		return a.writeJSON(out)
	}
	if m, ok := out.(map[string]any); ok {
		return a.detail(api.Object(m))
	}
	if out == nil {
		_, err := fmt.Fprintln(a.out, "Done.")
		return err
	}
	return a.writeJSON(out)
}

func terminal(v any) bool        { f, ok := v.(*os.File); return ok && term.IsTerminal(int(f.Fd())) }
func (a *app) interactive() bool { return terminal(a.in) && terminal(a.out) && !a.json && !a.plain }

func (a *app) prompt(label, fallback string) (string, error) {
	if !terminal(a.in) {
		return "", fmt.Errorf("%s is required in non-interactive mode; supply the corresponding flag", label)
	}
	if fallback != "" {
		fmt.Fprintf(a.errOut, "%s [%s]: ", label, Safe(fallback))
	} else {
		fmt.Fprintf(a.errOut, "%s: ", label)
	}
	s, err := a.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = fallback
	}
	return s, nil
}

func (a *app) confirm(label string) error {
	if a.yes {
		return nil
	}
	if !terminal(a.in) {
		return fmt.Errorf("%s requires --yes in non-interactive mode", label)
	}
	s, err := a.prompt(label+"? Type yes to confirm", "")
	if err != nil {
		return err
	}
	if s != "yes" {
		return fmt.Errorf("cancelled")
	}
	return nil
}

// Safe prevents remote text from injecting terminal control sequences.
func Safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, s)
}

func query() url.Values { return url.Values{} }
