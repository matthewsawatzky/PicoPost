package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "picopost.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	path := writeConfig(t, "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen != "127.0.0.1:8080" {
		t.Errorf("default listen = %q", cfg.Server.Listen)
	}
	if cfg.Posts.MaxBodyBytes != 1024 {
		t.Errorf("default max_body_bytes = %d", cfg.Posts.MaxBodyBytes)
	}
	if cfg.Posts.MaxTextBytes != 768 {
		t.Errorf("default max_text_bytes = %d", cfg.Posts.MaxTextBytes)
	}
	if !cfg.Identity.Anonymous {
		t.Error("anonymous should default to true")
	}
}

func TestLoadOverrides(t *testing.T) {
	path := writeConfig(t, `
[server]
listen = "0.0.0.0:9000"

[posts]
max_body_bytes = 2048
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Listen != "0.0.0.0:9000" {
		t.Errorf("listen = %q", cfg.Server.Listen)
	}
	if cfg.Posts.MaxBodyBytes != 2048 {
		t.Errorf("max_body_bytes = %d", cfg.Posts.MaxBodyBytes)
	}
	if cfg.Posts.MaxTextBytes != 768 {
		t.Errorf("max_text_bytes should keep default, got %d", cfg.Posts.MaxTextBytes)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadUnknownKey(t *testing.T) {
	path := writeConfig(t, "[server]\nlisten = \"x\"\nunknown_key = 1\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestLoadInvalidValues(t *testing.T) {
	cases := []string{
		"[server]\nlisten = \"\"\n",
		"[posts]\nmax_body_bytes = 0\n",
		"[posts]\nmax_text_bytes = 2000\nmax_body_bytes = 1000\n",
		"[cors]\norigins = [\"not-a-url\"]\n",
		"[cors]\norigins = [\"https://example.com/path\"]\n",
		"[rate_limit]\nposts_per_minute = -1\n",
	}
	for _, c := range cases {
		path := writeConfig(t, c)
		if _, err := Load(path); err == nil {
			t.Errorf("expected validation error for config:\n%s", c)
		}
	}
}

func TestLoadWildcardOrigin(t *testing.T) {
	path := writeConfig(t, "[cors]\norigins = [\"*\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CORS.Origins) != 1 || cfg.CORS.Origins[0] != "*" {
		t.Errorf("origins = %v", cfg.CORS.Origins)
	}
}

func TestLoadFilters(t *testing.T) {
	path := writeConfig(t, `
[filters.username]
deny = ["admin", "root"]

[filters.text]
deny = ["spam"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Filters.Username.Deny) != 2 || cfg.Filters.Username.Deny[0] != "admin" {
		t.Errorf("username deny = %v", cfg.Filters.Username.Deny)
	}
	if len(cfg.Filters.Text.Deny) != 1 || cfg.Filters.Text.Deny[0] != "spam" {
		t.Errorf("text deny = %v", cfg.Filters.Text.Deny)
	}
}

func TestValidateErrorMessages(t *testing.T) {
	path := writeConfig(t, "[posts]\nmax_body_bytes = 0\n")
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "max_body_bytes") {
		t.Errorf("error should mention the offending key, got: %v", err)
	}
}
