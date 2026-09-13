package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lfreixial/plane-cli/internal/api"
	"github.com/lfreixial/plane-cli/internal/config"
	"github.com/spf13/cobra"
)

func (a *app) projectUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [REF]",
		Short: "Select the default project (interactive when REF is omitted)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !a.interactive() {
				return fmt.Errorf("project reference required outside an interactive terminal: run 'plane project list', then 'plane project use KEY'")
			}
			if err := a.connect(); err != nil {
				return err
			}
			var id, name string
			if len(args) == 0 {
				projects, err := a.client.List(cmd.Context(), a.ws()+"projects/", nil, 0)
				if err != nil {
					return err
				}
				selected, err := a.pickProject(cmd, projects)
				if err != nil {
					return err
				}
				id, name = selected.String("id"), selected.String("name")
			} else {
				var err error
				name = args[0]
				id, err = a.resolve(cmd, a.ws()+"projects/", name, "project")
				if err != nil {
					return err
				}
			}
			if !uuid.MatchString(id) {
				return fmt.Errorf("API returned a project without a valid ID")
			}
			c := a.stored
			c.BaseURL = a.cfg.BaseURL
			c.WebURL = a.cfg.WebURL
			c.Workspace = a.cfg.Workspace
			c.Project = id
			if err := config.Save(a.path, c); err != nil {
				return err
			}
			if a.json {
				return a.writeJSON(map[string]string{"project": id})
			}
			_, err := fmt.Fprintf(a.out, "Default project: %s (%s)\n", Safe(name), id)
			return err
		},
	}
}

type projectItem struct{ project api.Object }

func (p projectItem) Title() string {
	return Safe(strings.TrimSpace(p.project.String("identifier") + "  " + p.project.String("name")))
}
func (p projectItem) Description() string { return Safe(p.project.String("id")) }
func (p projectItem) FilterValue() string { return p.Title() + " " + p.Description() }

type projectPicker struct {
	list     list.Model
	selected api.Object
}

func newProjectPicker(projects []api.Object, current string) projectPicker {
	items := make([]list.Item, len(projects))
	index := 0
	for i, project := range projects {
		items[i] = projectItem{project}
		for _, field := range []string{"id", "identifier", "name"} {
			if current != "" && strings.EqualFold(project.String(field), current) {
				index = i
				break
			}
		}
	}
	l := list.New(items, list.NewDefaultDelegate(), 80, 22)
	l.Title = "Plane · Select a project"
	l.SetStatusBarItemName("project", "projects")
	l.Select(index)
	return projectPicker{list: l}
}

func (p projectPicker) Init() tea.Cmd { return nil }

func (p projectPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		p.list.SetSize(max(1, m.Width), max(1, m.Height-2))
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return p, tea.Quit
		}
		if m.String() == "enter" && p.list.FilterState() != list.Filtering {
			if item, ok := p.list.SelectedItem().(projectItem); ok {
				p.selected = item.project
				return p, tea.Quit
			}
		}
		if m.String() == "esc" && p.list.FilterState() == list.Unfiltered {
			return p, tea.Quit
		}
	}
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p projectPicker) View() string { return p.list.View() + "\n  enter select • esc cancel\n" }

func (a *app) pickProject(cmd *cobra.Command, projects []api.Object) (api.Object, error) {
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects found: create one with 'plane project create'")
	}
	m, err := tea.NewProgram(newProjectPicker(projects, a.cfg.Project),
		tea.WithContext(cmd.Context()), tea.WithInput(a.in), tea.WithOutput(a.out), tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	selected := m.(projectPicker).selected
	if selected == nil {
		return nil, fmt.Errorf("project selection cancelled")
	}
	return selected, nil
}
