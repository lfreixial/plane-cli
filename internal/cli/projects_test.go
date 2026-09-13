package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lfreixial/plane-cli/internal/api"
	"github.com/spf13/cobra"
)

func TestProjectViewDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"selected", nil, projectID},
		{"flag", []string{"--project", otherProjectID}, otherProjectID},
		{"explicit", []string{otherProjectID}, otherProjectID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
				want := "/api/v1/workspaces/team/projects/" + tc.want + "/"
				if r.Method != "GET" || r.URL.Path != want {
					t.Errorf("request=%s %s, want GET %s", r.Method, r.URL.Path, want)
				}
				fmt.Fprintf(w, `{"id":%q,"name":"Engineering"}`, tc.want)
			})
			args := append([]string{"project", "view", "--json"}, tc.args...)
			out, err := invoke(t, "", args...)
			if err != nil {
				t.Fatal(err)
			}
			var project api.Object
			if err := json.Unmarshal([]byte(out), &project); err != nil {
				t.Fatal(err)
			}
			if project.String("id") != tc.want {
				t.Fatal(project)
			}
		})
	}
}

func TestProjectViewNoDefaultIsActionable(t *testing.T) {
	calls := 0
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	t.Setenv("PLANE_PROJECT", "")
	_, err := invoke(t, "", "project", "view")
	if err == nil || !strings.Contains(err.Error(), "plane project use") {
		t.Fatalf("error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("unexpected requests: %d", calls)
	}
}

func TestProjectUseRequiresReferenceInScripts(t *testing.T) {
	calls := 0
	path := serverEnv(t, func(w http.ResponseWriter, r *http.Request) { calls++ })
	for _, flags := range [][]string{nil, {"--json"}, {"--plain"}} {
		_, err := invoke(t, "", append([]string{"project", "use"}, flags...)...)
		if err == nil || !strings.Contains(err.Error(), "plane project use KEY") {
			t.Fatalf("error=%v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("unexpected requests: %d", calls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config should not be created: %v", err)
	}
}

func TestProjectPickerSelectionAndCancellation(t *testing.T) {
	projects := []api.Object{
		{"id": projectID, "identifier": "ENG", "name": "Engineering"},
		{"id": otherProjectID, "identifier": "OPS", "name": "Operations"},
	}
	for _, current := range []string{otherProjectID, "ops", "operations"} {
		p := newProjectPicker(projects, current)
		m, quit := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.(projectPicker).selected.String("id") != otherProjectID || quit == nil {
			t.Fatalf("failed to select current project %q", current)
		}
	}
	p := newProjectPicker(projects, "")
	m, _ := p.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, quit := m.(projectPicker).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.(projectPicker).selected.String("id") != otherProjectID || quit == nil {
		t.Fatal("down/enter did not select second project")
	}
	for _, key := range []tea.KeyType{tea.KeyEscape, tea.KeyCtrlC} {
		m, quit := p.Update(tea.KeyMsg{Type: key})
		if m.(projectPicker).selected != nil || quit == nil {
			t.Fatal("cancel selected a project")
		}
	}
	m, _ = p.Update(tea.WindowSizeMsg{Width: 1, Height: 1})
	if !strings.Contains(m.(projectPicker).View(), "enter select") {
		t.Fatal("missing selection instructions")
	}
}

func TestProjectPickerEmptyWorkspace(t *testing.T) {
	a := &app{}
	_, err := a.pickProject(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "plane project create") {
		t.Fatalf("error=%v", err)
	}
}
