package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func clientFor(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c, err := New(s.URL, "test-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPaginationAndAuthentication(t *testing.T) {
	calls := 0
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-API-Key") != "test-secret" {
			t.Error("missing authentication")
		}
		if r.URL.Path != "/api/v1/workspaces/team/projects/" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "3" {
			t.Error("wrong page size")
		}
		if r.URL.Query().Get("order_by") != "-created_at" {
			t.Error("query lost")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"results":[{"id":"1"},{"id":"2"}],"next_cursor":"2:1:0","next_page_results":true}`)
		} else {
			if r.URL.Query().Get("cursor") != "2:1:0" {
				t.Error("cursor lost")
			}
			fmt.Fprint(w, `{"results":[{"id":"3"},{"id":"4"}],"next_page_results":false}`)
		}
	})
	q := url.Values{"order_by": {"-created_at"}}
	items, err := c.List(context.Background(), "workspaces/team/projects/", q, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[2].String("id") != "3" || calls != 2 {
		t.Fatalf("items=%v calls=%d", items, calls)
	}
	if q.Get("cursor") != "" || q.Get("per_page") != "" {
		t.Fatal("mutated caller query")
	}
}

func TestListResponseShapes(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantErr    bool
		count      int
	}{
		{"array", `[{"id":"a"}]`, false, 1}, {"empty array", `[]`, false, 0},
		{"empty page", `{"results":[],"next_page_results":false}`, false, 0},
		{"malformed", `<html>bad gateway</html>`, true, 0}, {"unexpected", `{"data":[]}`, true, 0},
		{"missing cursor", `{"results":[],"next_page_results":true}`, true, 0},
		{"repeated cursor", `{"results":[],"next_page_results":true,"next_cursor":"same"}`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, tc.body) })
			items, err := c.List(context.Background(), "projects/", nil, 0)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			if !tc.wantErr && len(items) != tc.count {
				t.Fatalf("items=%v", items)
			}
		})
	}
}

func TestMutationPayloadAndNoContent(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("request headers/method")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(body, map[string]any{"assignees": []any{}, "target_date": nil}) {
			t.Errorf("payload: %#v", body)
		}
		w.WriteHeader(204)
	})
	if err := c.Do(context.Background(), "PATCH", "work-items/id/", nil, map[string]any{"assignees": []string{}, "target_date": nil}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRateLimitRetriesReadsOnly(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			calls := 0
			c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(429)
					return
				}
				fmt.Fprint(w, `{}`)
			})
			err := c.Do(context.Background(), method, "test/", nil, nil, nil)
			if method == "GET" && (err != nil || calls != 2) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if method == "POST" && (err == nil || calls != 1) {
				t.Fatalf("mutation replayed: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestErrorsAndRedaction(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"detail":"test-secret denied"}`)
	})
	err := c.Do(context.Background(), "GET", "test/", nil, nil, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 403 {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(err.Error(), "test-secret") || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("unsafe/unhelpful error=%v", err)
	}
}

func TestRedirectDoesNotLeakKey(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) })
	if err := c.Do(context.Background(), "GET", "test/", nil, nil, nil); err == nil {
		t.Fatal("expected redirect error")
	}
	if hits.Load() != 0 {
		t.Fatal("followed credential-bearing redirect")
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c.HTTP.Timeout = 20 * time.Millisecond
	if err := c.Do(context.Background(), "GET", "test/", nil, nil, nil); err == nil {
		t.Fatal("expected timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Do(ctx, "GET", "test/", nil, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestBaseURLValidation(t *testing.T) {
	for _, raw := range []string{"file:///tmp/key", "https://user:pass@example.com", "https://example.com?secret=x", "https://example.com#x", "localhost:3000"} {
		if _, err := New(raw, "key", time.Second); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"https://example.com", "https://example.com/api/v1/", "http://localhost:3000/plane"} {
		c, err := New(raw, "key", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(c.base.Path, "/api/v1") != 1 {
			t.Errorf("path=%s", c.base.Path)
		}
	}
}
