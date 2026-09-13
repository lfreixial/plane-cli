// Package api implements Plane's public v1 API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Object map[string]any

func (o Object) String(key string) string {
	v := o[key]
	if v == nil {
		return ""
	}
	if items, ok := v.([]any); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			parts = append(parts, Object{"value": item}.String("value"))
		}
		return strings.Join(parts, ", ")
	}
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"name", "display_name", "email", "id"} {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
	}
	return fmt.Sprint(v)
}

func (o Object) ID(key string) string {
	if m, ok := o[key].(map[string]any); ok {
		s, _ := m["id"].(string)
		return s
	}
	s, _ := o[key].(string)
	return s
}

type Client struct {
	base *url.URL
	key  string
	HTTP *http.Client
}

func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("URL must be an http(s) origin or base path, without credentials, query, or fragment")
	}
	return u, nil
}

func New(base, key string, timeout time.Duration) (*Client, error) {
	u, err := ValidateURL(base)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("missing API key: run 'plane init' or set PLANE_API_KEY")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(u.Path, "/api/v1") {
		u.Path += "/api/v1"
	}
	u.RawPath = ""
	return &Client{base: u, key: key, HTTP: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type Error struct {
	Status                           int
	Method, Path, Detail, RetryAfter string
}

func (e *Error) Error() string {
	s := fmt.Sprintf("Plane API %s %s: %d %s", e.Method, e.Path, e.Status, e.Detail)
	switch e.Status {
	case 401:
		s += " (check your API key)"
	case 403:
		s += " (check workspace membership and token permissions)"
	case 404:
		s += " (check workspace, project, resource ID, and server API version)"
	case 429:
		s += " (rate limited; retry later"
		if e.RetryAfter != "" {
			s += ", Retry-After: " + e.RetryAfter
		}
		s += ")"
	}
	return s
}

func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	u := *c.base
	u.Path += "/" + strings.TrimLeft(path, "/")
	u.RawQuery = query.Encode()
	// Only GET is retried: replaying mutations could create duplicate work items.
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("X-API-Key", c.key)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "plane-cli")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if len(data) > 16<<20 {
			return fmt.Errorf("API response exceeds 16 MiB")
		}
		if resp.StatusCode == 429 && method == http.MethodGet && attempt < 2 {
			delay := time.Duration(attempt+1) * time.Second
			if v := resp.Header.Get("Retry-After"); v != "" {
				if n, e := strconv.Atoi(v); e == nil {
					delay = time.Duration(n) * time.Second
				} else if t, e := http.ParseTime(v); e == nil {
					delay = time.Until(t)
				}
			}
			if delay < 0 {
				delay = 0
			}
			if delay <= 5*time.Second {
				t := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-t.C:
				}
				continue
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			detail := strings.TrimSpace(string(data))
			detail = strings.ReplaceAll(detail, c.key, "[redacted]")
			if len(detail) > 1024 {
				detail = detail[:1024] + "…"
			}
			return &Error{resp.StatusCode, method, path, detail, resp.Header.Get("Retry-After")}
		}
		if out == nil || len(bytes.TrimSpace(data)) == 0 {
			return nil
		}
		if err = json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("invalid API JSON: %w", err)
		}
		return nil
	}
}

// List accepts both cursor envelopes and unpaginated arrays. A zero limit fetches all pages.
func (c *Client) List(ctx context.Context, path string, query url.Values, limit int) ([]Object, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}
	q := url.Values{}
	for k, v := range query {
		q[k] = append([]string(nil), v...)
	}
	pageSize := 100
	if limit > 0 && limit < pageSize {
		pageSize = limit
	}
	q.Set("per_page", strconv.Itoa(pageSize))
	items := []Object{}
	seen := map[string]bool{}
	for {
		var raw json.RawMessage
		if err := c.Do(ctx, "GET", path, q, nil, &raw); err != nil {
			return nil, err
		}
		raw = bytes.TrimSpace(raw)
		var batch []Object
		var next string
		var more bool
		if len(raw) > 0 && raw[0] == '[' {
			if err := json.Unmarshal(raw, &batch); err != nil {
				return nil, err
			}
		} else {
			var page struct {
				Results []Object `json:"results"`
				Next    string   `json:"next_cursor"`
				More    bool     `json:"next_page_results"`
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				return nil, err
			}
			if page.Results == nil {
				return nil, fmt.Errorf("unexpected list response: missing results array")
			}
			batch, next, more = page.Results, page.Next, page.More
		}
		items = append(items, batch...)
		if limit > 0 && len(items) >= limit {
			return items[:limit], nil
		}
		if !more {
			return items, nil
		}
		if next == "" || seen[next] {
			return nil, fmt.Errorf("invalid pagination: missing or repeated next cursor")
		}
		seen[next] = true
		q.Set("cursor", next)
	}
}
