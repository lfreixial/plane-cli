package cli

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lfreixial/plane-cli/internal/api"
	"github.com/spf13/cobra"
)

type issueItem struct{ o api.Object }

func (i issueItem) Title() string { return Safe(i.o.String("key") + "  " + i.o.String("name")) }
func (i issueItem) Description() string {
	return Safe(i.o.String("state") + " • " + i.o.String("priority"))
}
func (i issueItem) FilterValue() string { return i.Title() + " " + i.Description() }

type browser struct {
	list     list.Model
	viewport viewport.Model
	selected api.Object
	detail   bool
}

func (b browser) Init() tea.Cmd { return nil }
func (b browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		b.list.SetSize(m.Width, m.Height-2)
		b.viewport.Width = max(1, m.Width-4)
		b.viewport.Height = max(1, m.Height-4)
		if b.detail {
			b.viewport.SetContent(ansi.Hardwrap(detailText(b.selected), b.viewport.Width, true))
		}
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return b, tea.Quit
		}
		if b.detail {
			if m.String() == "esc" {
				b.detail = false
				return b, nil
			}
			if m.String() == "q" {
				return b, tea.Quit
			}
		} else if b.list.FilterState() != list.Filtering && m.String() == "enter" {
			if i, ok := b.list.SelectedItem().(issueItem); ok {
				b.selected = i.o
				b.detail = true
				b.viewport.SetContent(ansi.Hardwrap(detailText(i.o), b.viewport.Width, true))
				b.viewport.GotoTop()
			}
			return b, nil
		}
	}
	var cmd tea.Cmd
	if b.detail {
		b.viewport, cmd = b.viewport.Update(msg)
	} else {
		b.list, cmd = b.list.Update(msg)
	}
	return b, cmd
}
func (b browser) View() string {
	if b.detail {
		return "\n" + b.viewport.View() + "\n\n  ↑/↓ scroll • esc back • q quit\n"
	}
	return b.list.View() + "\n  enter view work item\n"
}

func (a *app) browse(cmd *cobra.Command, items []api.Object) error {
	rows := make([]list.Item, len(items))
	for i, o := range items {
		rows[i] = issueItem{o}
	}
	l := list.New(rows, list.NewDefaultDelegate(), 80, 22)
	l.Title = "Plane · Work items"
	l.SetStatusBarItemName("work item", "work items")
	m := browser{list: l, viewport: viewport.New(76, 20)}
	_, err := tea.NewProgram(m, tea.WithContext(cmd.Context()), tea.WithInput(a.in), tea.WithOutput(a.out), tea.WithAltScreen()).Run()
	return err
}
