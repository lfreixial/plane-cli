// Package config handles persisted defaults and environment overrides.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	BaseURL   string `json:"base_url"`
	WebURL    string `json:"web_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	Workspace string `json:"workspace"`
	Project   string `json:"project,omitempty"`
}

func Path() (string, error) {
	if p := os.Getenv("PLANE_CONFIG"); p != "" {
		return p, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "plane-cli", "config.json"), nil
}

func Load(path string) (Config, error) {
	c := Config{BaseURL: "https://api.plane.so"}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config %s: %w", path, err)
	}
	return c, nil
}

func (c Config) WithEnv() Config {
	for key, ptr := range map[string]*string{"PLANE_URL": &c.BaseURL, "PLANE_WEB_URL": &c.WebURL, "PLANE_API_KEY": &c.APIKey, "PLANE_WORKSPACE": &c.Workspace, "PLANE_PROJECT": &c.Project} {
		if v, ok := os.LookupEnv(key); ok {
			*ptr = v
		}
	}
	return c
}

// Save replaces the file atomically. API keys are only persisted by explicit init.
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".plane-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
