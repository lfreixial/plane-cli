package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/lfreixial/plane-cli/internal/api"
	"github.com/lfreixial/plane-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func (a *app) initCmd() *cobra.Command {
	var noInput, saveToken bool
	c := &cobra.Command{Use: "init", Short: "Configure Plane and verify workspace access", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var err error
		if !noInput {
			if !cmd.Flags().Changed("url") {
				a.cfg.BaseURL, err = a.prompt("API URL", a.cfg.BaseURL)
				if err != nil {
					return err
				}
			}
			if a.cfg.Workspace == "" {
				a.cfg.Workspace, err = a.prompt("Workspace slug", "")
				if err != nil {
					return err
				}
			}
			if a.cfg.APIKey == "" {
				f, ok := a.in.(*os.File)
				if !ok || !terminal(a.in) {
					return fmt.Errorf("set PLANE_API_KEY for non-interactive setup")
				}
				fmt.Fprint(a.errOut, "API key (hidden): ")
				b, e := term.ReadPassword(int(f.Fd()))
				fmt.Fprintln(a.errOut)
				if e != nil {
					return e
				}
				a.cfg.APIKey = strings.TrimSpace(string(b))
				saveToken = true
			}
		}
		if a.cfg.WebURL != "" {
			if _, err := api.ValidateURL(a.cfg.WebURL); err != nil {
				return fmt.Errorf("web URL: %w", err)
			}
		}
		if err = a.connect(); err != nil {
			return err
		}
		projects, err := a.client.List(cmd.Context(), a.ws()+"projects/", nil, 0)
		if err != nil {
			return err
		}
		if a.cfg.Project == "" && len(projects) > 0 && !noInput {
			if err := a.table(projects, []string{"identifier", "name", "id"}); err != nil {
				return err
			}
			a.cfg.Project, err = a.prompt("Default project (optional)", "")
			if err != nil {
				return err
			}
		}
		if a.cfg.Project != "" {
			if _, err = a.projectID(cmd); err != nil {
				return err
			}
		}
		saved := a.cfg
		// Environment keys stay in the environment unless the caller opts in.
		if !saveToken {
			saved.APIKey = a.stored.APIKey
		}
		if err = config.Save(a.path, saved); err != nil {
			return err
		}
		if a.json {
			return a.writeJSON(map[string]any{"config": a.path, "workspace": saved.Workspace, "project": saved.Project})
		}
		fmt.Fprintf(a.out, "Configured %s in %s\n", Safe(saved.Workspace), a.path)
		return nil
	}}
	c.Flags().BoolVar(&noInput, "no-input", false, "Use flags and environment without prompting")
	c.Flags().BoolVar(&saveToken, "save-token", false, "Persist PLANE_API_KEY in the private config file")
	return c
}

func (a *app) configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Inspect configuration"}
	c.AddCommand(&cobra.Command{Use: "show", Short: "Show effective configuration with the API key redacted", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		v := a.cfg
		if v.APIKey != "" {
			v.APIKey = "[redacted]"
		}
		return a.writeJSON(v)
	}})
	c.AddCommand(&cobra.Command{Use: "path", Short: "Print the config path", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { _, err := fmt.Fprintln(a.out, a.path); return err }})
	return c
}
