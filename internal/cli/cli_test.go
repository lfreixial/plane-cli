package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lfreixial/plane-cli/internal/api"
	"github.com/lfreixial/plane-cli/internal/config"
)

const projectID = "11111111-1111-4111-8111-111111111111"
const workID = "22222222-2222-4222-8222-222222222222"
const stateID = "33333333-3333-4333-8333-333333333333"
const memberID = "44444444-4444-4444-8444-444444444444"
const cycleID = "55555555-5555-4555-8555-555555555555"
const otherProjectID = "66666666-6666-4666-8666-666666666666"
const projectPath = "/api/v1/workspaces/team/projects/" + projectID + "/"

func environment(t *testing.T) string {
	t.Helper()
	for _, k := range []string{"PLANE_URL", "PLANE_WEB_URL", "PLANE_API_KEY", "PLANE_WORKSPACE", "PLANE_PROJECT"} {
		t.Setenv(k, "")
	}
	p := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PLANE_CONFIG", p)
	return p
}

func invoke(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	r := New("test", strings.NewReader(input), &out, &errOut)
	r.SetArgs(args)
	err := r.ExecuteContext(context.Background())
	return out.String(), err
}

func serverEnv(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	path := environment(t)
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	t.Setenv("PLANE_URL", s.URL)
	t.Setenv("PLANE_API_KEY", "secret")
	t.Setenv("PLANE_WORKSPACE", "team")
	t.Setenv("PLANE_PROJECT", projectID)
	return path
}

func issueResponse(w http.ResponseWriter) {
	fmt.Fprintf(w, `{"id":%q,"project":%q,"name":"Fix login","sequence_id":42}`, workID, projectID)
}

func TestCreateResolvesNamesAndEscapesBody(t *testing.T) {
	var mutation map[string]any
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			t.Error("missing API key")
		}
		switch r.URL.Path {
		case "/api/v1/workspaces/team/projects/":
			fmt.Fprintf(w, `[{"id":%q,"identifier":"ENG","name":"Engineering"}]`, projectID)
		case projectPath + "states/":
			fmt.Fprintf(w, `[{"id":%q,"name":"In Progress"}]`, stateID)
		case projectPath + "project-members/":
			fmt.Fprintf(w, `[{"id":%q,"email":"alice@example.com"}]`, memberID)
		case projectPath + "work-items/":
			if r.Method != "POST" {
				t.Error("expected POST")
			}
			if err := json.NewDecoder(r.Body).Decode(&mutation); err != nil {
				t.Error(err)
			}
			w.WriteHeader(201)
			issueResponse(w)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	out, err := invoke(t, "hello <script>\nworld", "issue", "create", "-p", "eng", "-t", "Fix login", "--body-file", "-", "-s", "In Progress", "-a", "alice@example.com", "--priority", "high", "--json")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"name": "Fix login", "description_html": "<p>hello &lt;script&gt;<br>world</p>", "state": stateID, "assignees": []any{memberID}, "priority": "high"}
	if !reflect.DeepEqual(mutation, want) {
		t.Fatalf("payload=%#v", mutation)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("JSON output polluted: %s", out)
	}
}

func TestMoveKeyUsesActualProject(t *testing.T) {
	patched := false
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspaces/team/work-items/ENG-42/":
			issueResponse(w)
		case projectPath + "states/":
			fmt.Fprintf(w, `[{"id":%q,"name":"Done"}]`, stateID)
		case projectPath + "work-items/" + workID + "/":
			if r.Method != "PATCH" {
				t.Error("expected PATCH")
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if !reflect.DeepEqual(body, map[string]any{"state": stateID}) {
				t.Errorf("payload=%v", body)
			}
			patched = true
			issueResponse(w)
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
		}
	})
	_, err := invoke(t, "", "issue", "move", "eng-42", "Done", "-p", otherProjectID)
	if err != nil || !patched {
		t.Fatalf("patched=%v err=%v", patched, err)
	}
}

func TestEditClearFieldsAndPreserveUnchanged(t *testing.T) {
	var payload map[string]any
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			json.NewDecoder(r.Body).Decode(&payload)
		}
		issueResponse(w)
	})
	_, err := invoke(t, "", "issue", "edit", "ENG-42", "--assignee", "none", "--label", "none", "--target", "none", "--body=", "--json")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"assignees": []any{}, "labels": []any{}, "target_date": nil, "description_html": "<p></p>"}
	if !reflect.DeepEqual(payload, want) {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestFilteredListFindsMatchesOnLaterPages(t *testing.T) {
	calls := 0
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("expand") != "state,assignees,labels,project" {
			t.Error("missing expand")
		}
		if r.URL.Query().Get("cursor") == "" {
			fmt.Fprint(w, `{"results":[{"id":"first","name":"Other task","priority":"low"}],"next_page_results":true,"next_cursor":"100:1:0"}`)
		} else {
			fmt.Fprint(w, `{"results":[{"id":"second","name":"Login bug","priority":"high","sequence_id":42,"project":{"identifier":"ENG"}}],"next_page_results":false}`)
		}
	})
	out, err := invoke(t, "", "issue", "list", "--priority", "high", "--search", "LOGIN", "--limit", "1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var items []api.Object
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(items) != 1 || items[0].String("key") != "ENG-42" {
		t.Fatalf("calls=%d items=%v", calls, items)
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	deleted := 0
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deleted++
			w.WriteHeader(204)
		} else {
			issueResponse(w)
		}
	})
	if _, err := invoke(t, "", "issue", "delete", "ENG-42"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error=%v", err)
	}
	if deleted != 0 {
		t.Fatal("deleted without confirmation")
	}
	out, err := invoke(t, "", "issue", "delete", "ENG-42", "--yes", "--json")
	if err != nil || deleted != 1 || strings.TrimSpace(out) != "null" {
		t.Fatalf("deleted=%d out=%q err=%v", deleted, out, err)
	}
}

func TestCycleMembershipAndProjectGuard(t *testing.T) {
	posts := 0
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspaces/team/work-items/ENG-42/":
			issueResponse(w)
		case projectPath + "cycles/" + cycleID + "/cycle-issues/":
			if r.Method != "POST" {
				t.Error("expected POST")
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if !reflect.DeepEqual(body, map[string]any{"issues": []any{workID}}) {
				t.Errorf("payload=%v", body)
			}
			posts++
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected %s", r.URL)
			http.NotFound(w, r)
		}
	})
	if _, err := invoke(t, "", "cycle", "add", cycleID, "ENG-42"); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke(t, "", "cycle", "add", cycleID, "ENG-42", "-p", otherProjectID); err == nil || !strings.Contains(err.Error(), "different project") {
		t.Fatalf("error=%v", err)
	}
	if posts != 1 {
		t.Fatalf("posts=%d", posts)
	}
}

func TestCommentEscapesText(t *testing.T) {
	var body map[string]any
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if r.URL.Path != projectPath+"work-items/"+workID+"/comments/" {
				t.Errorf("path=%s", r.URL.Path)
			}
			json.NewDecoder(r.Body).Decode(&body)
			fmt.Fprint(w, `{"id":"comment"}`)
		} else {
			issueResponse(w)
		}
	})
	if _, err := invoke(t, "", "issue", "comment", "add", "ENG-42", "A & B <test>"); err != nil {
		t.Fatal(err)
	}
	if body["comment_html"] != "<p>A &amp; B &lt;test&gt;</p>" {
		t.Fatal(body)
	}
}

func TestInitDoesNotPersistEnvironmentTokenByDefault(t *testing.T) {
	p := serverEnv(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `[]`) })
	_, err := invoke(t, "", "init", "--no-input", "--json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "" {
		t.Fatal("environment key persisted")
	}
	_, err = invoke(t, "", "init", "--no-input", "--save-token")
	if err != nil {
		t.Fatal(err)
	}
	c, _ = config.Load(p)
	if c.APIKey != "secret" {
		t.Fatal("explicit key persistence failed")
	}
}

func TestConfigPrecedenceAndRedaction(t *testing.T) {
	p := environment(t)
	if err := config.Save(p, config.Config{BaseURL: "https://stored.example", Workspace: "stored", APIKey: "stored-secret"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLANE_URL", "https://env.example")
	t.Setenv("PLANE_WORKSPACE", "env")
	t.Setenv("PLANE_API_KEY", "env-secret")
	out, err := invoke(t, "", "config", "show", "--workspace", "flag")
	if err != nil {
		t.Fatal(err)
	}
	var c config.Config
	json.Unmarshal([]byte(out), &c)
	if c.BaseURL != "https://env.example" || c.Workspace != "flag" || c.APIKey != "[redacted]" {
		t.Fatalf("config=%+v", c)
	}
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "env-secret") {
		t.Fatal("environment persisted")
	}
}

func TestInvalidInputMakesNoRequests(t *testing.T) {
	calls := 0
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unexpected", 500) })
	for _, args := range [][]string{
		{"issue", "create", "--title", "x", "--priority", "critical"},
		{"issue", "create", "--title", "x", "--start", "tomorrow"},
		{"issue", "create", "--title", "x", "--body", "x", "--body-file", "-"},
		{"issue", "edit", "ENG-42"}, {"issue", "list", "--limit", "-1"}, {"issue", "view", "../../bad"},
	} {
		if _, err := invoke(t, "", args...); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	if calls != 0 {
		t.Fatalf("made %d requests", calls)
	}
}

func TestAmbiguousStateRefFailsBeforeMutation(t *testing.T) {
	patches := 0
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			patches++
		}
		if strings.HasSuffix(r.URL.Path, "states/") {
			fmt.Fprint(w, `[{"id":"a","name":"Done"},{"id":"b","name":"Done"}]`)
		} else {
			issueResponse(w)
		}
	})
	_, err := invoke(t, "", "issue", "move", "ENG-42", "Done")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || patches != 0 {
		t.Fatalf("patches=%d err=%v", patches, err)
	}
}

func TestPlainOutputAndHelp(t *testing.T) {
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"bad\u001b[2J\tname","sequence_id":1}]`)
	})
	out, err := invoke(t, "", "issue", "list", "--plain")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\x1b") || !strings.Contains(out, "NAME") {
		t.Fatalf("output=%q", out)
	}
	out, err = invoke(t, "", "--help")
	if err != nil || !strings.Contains(out, "completion") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestBrowserNavigation(t *testing.T) {
	o := api.Object{"key": "ENG-42", "name": "Fix login", "description_stripped": strings.Repeat("long description ", 100)}
	b := browser{list: list.New([]list.Item{issueItem{o}}, list.NewDefaultDelegate(), 80, 24), viewport: viewport.New(76, 20)}
	m, _ := b.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	b = m.(browser)
	m, _ = b.Update(tea.KeyMsg{Type: tea.KeyEnter})
	b = m.(browser)
	if !b.detail || !strings.Contains(b.View(), "ENG-42") {
		t.Fatal("detail did not open")
	}
	m, _ = b.Update(tea.KeyMsg{Type: tea.KeyEscape})
	b = m.(browser)
	if b.detail {
		t.Fatal("escape did not return to list")
	}
	_, quit := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if quit == nil {
		t.Fatal("ctrl+c did not quit")
	}
}

func TestResourceCreatePayloads(t *testing.T) {
	for _, tc := range []struct {
		kind, path string
		flags      []string
		want       map[string]any
	}{
		{"project", "/api/v1/workspaces/team/projects/", []string{"--identifier", "eng"}, map[string]any{"name": "Example", "identifier": "ENG"}},
		{"state", projectPath + "states/", nil, map[string]any{"name": "Example", "group": "backlog", "color": "#60646C"}},
		{"label", projectPath + "labels/", []string{"--color", "#FF0000"}, map[string]any{"name": "Example", "color": "#FF0000"}},
		{"cycle", projectPath + "cycles/", []string{"--start", "2026-10-01", "--end", "2026-10-14"}, map[string]any{"name": "Example", "start_date": "2026-10-01", "end_date": "2026-10-14"}},
		{"module", projectPath + "modules/", []string{"--target", "2026-10-31", "--status", "planned"}, map[string]any{"name": "Example", "target_date": "2026-10-31", "status": "planned"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var got map[string]any
			serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != tc.path {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				fmt.Fprint(w, `{"id":"created"}`)
			})
			args := append([]string{tc.kind, "create", "--name", "Example", "--json"}, tc.flags...)
			if _, err := invoke(t, "", args...); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("payload=%#v want=%#v", got, tc.want)
			}
		})
	}
}

func TestResourceListViewEditDelete(t *testing.T) {
	for _, kind := range []string{"project", "state", "label", "cycle", "module"} {
		t.Run(kind, func(t *testing.T) {
			path := projectPath + kind + "s/"
			if kind == "project" {
				path = "/api/v1/workspaces/team/projects/"
			}
			var methods []string
			serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.URL.Path != path && r.URL.Path != path+cycleID+"/" {
					t.Errorf("path=%s", r.URL.Path)
				}
				if r.Method == "PATCH" {
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if !reflect.DeepEqual(body, map[string]any{"description": "Changed"}) {
						t.Errorf("payload=%v", body)
					}
				}
				if r.Method == "DELETE" {
					w.WriteHeader(204)
					return
				}
				if r.URL.Path == path {
					fmt.Fprintf(w, `[{"id":%q,"name":"Example"}]`, cycleID)
				} else {
					fmt.Fprintf(w, `{"id":%q,"name":"Example"}`, cycleID)
				}
			})
			for _, args := range [][]string{{kind, "list", "--json"}, {kind, "view", cycleID, "--json"}, {kind, "edit", cycleID, "--description", "Changed", "--json"}, {kind, "delete", cycleID, "--yes", "--json"}} {
				if _, err := invoke(t, "", args...); err != nil {
					t.Fatalf("%v: %v", args, err)
				}
			}
			if !reflect.DeepEqual(methods, []string{"GET", "GET", "PATCH", "DELETE"}) {
				t.Fatal(methods)
			}
		})
	}
}

func TestModuleMembershipRemoveAndList(t *testing.T) {
	path := projectPath + "modules/" + cycleID + "/module-issues/"
	var deleted string
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deleted = r.URL.Path
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == path {
			fmt.Fprintf(w, `[{"id":"membership","issue":%q}]`, workID)
		} else {
			issueResponse(w)
		}
	})
	if _, err := invoke(t, "", "module", "issues", cycleID, "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke(t, "", "module", "remove", cycleID, "ENG-42"); err != nil {
		t.Fatal(err)
	}
	if deleted != path+workID+"/" {
		t.Fatalf("deleted=%s", deleted)
	}
}

func TestProjectUseKeepsEnvironmentTokenOutOfConfig(t *testing.T) {
	p := serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"id":%q,"identifier":"ENG"}]`, projectID)
	})
	if _, err := invoke(t, "", "project", "use", "ENG"); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Project != projectID || c.Workspace != "team" || c.APIKey != "" {
		t.Fatalf("config=%+v", c)
	}
}

func TestMemberEndpointsAndBrowserURL(t *testing.T) {
	var paths []string
	serverEnv(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, "work-items/") {
			issueResponse(w)
		} else {
			fmt.Fprint(w, `[]`)
		}
	})
	for _, args := range [][]string{{"member", "list", "--json"}, {"member", "list", "--all", "--json"}} {
		if _, err := invoke(t, "", args...); err != nil {
			t.Fatal(err)
		}
	}
	out, err := invoke(t, "", "issue", "open", "ENG-42", "--web-url", "https://plane.example.com", "--print")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://plane.example.com/team/projects/" + projectID + "/issues/" + workID
	if strings.TrimSpace(out) != want {
		t.Fatalf("URL=%q", out)
	}
	if paths[0] != projectPath+"project-members/" || paths[1] != "/api/v1/workspaces/team/members/" {
		t.Fatal(paths)
	}
}
