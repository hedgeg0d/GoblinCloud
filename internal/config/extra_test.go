package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	// Flag wins over everything.
	if got := Resolve("/explicit/path.toml"); got != "/explicit/path.toml" {
		t.Errorf("flag: got %q", got)
	}

	// Env var used when no flag.
	t.Setenv("GOBLIN_CONFIG", "/from/env.toml")
	if got := Resolve(""); got != "/from/env.toml" {
		t.Errorf("env: got %q", got)
	}

	// With neither flag nor env, and no /etc file, falls back to ./config.toml.
	t.Setenv("GOBLIN_CONFIG", "")
	if _, err := os.Stat("/etc/goblin/config.toml"); os.IsNotExist(err) {
		if got := Resolve(""); got != "config.toml" {
			t.Errorf("default: got %q", got)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestLoadMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	os.WriteFile(path, []byte("this is = not = valid toml ]["), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error loading malformed toml")
	}
}

func TestParseSizeErrors(t *testing.T) {
	for _, in := range []string{"12x", "abc", "1.2.3GB", "GB", "-"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) expected error", in)
		}
	}
}

func TestValidateMoreBranches(t *testing.T) {
	good := t.TempDir()

	// Global mode without autocert_cache.
	c := Default()
	c.Auth.Enabled = false
	c.Storage.Paths = []string{good}
	c.Server.Domain = "x.com"
	c.Server.AutocertEmail = "a@x.com"
	c.Server.AutocertCache = ""
	if !hasProblem(c.Validate(), "autocert_cache") {
		t.Error("expected autocert_cache problem")
	}

	// Storage path points at a file, not a directory.
	file := filepath.Join(good, "afile")
	os.WriteFile(file, []byte("x"), 0o644)
	c = Default()
	c.Auth.Enabled = false
	c.Storage.Paths = []string{file}
	if !hasProblem(c.Validate(), "not a directory") {
		t.Error("expected not-a-directory problem")
	}

	// Empty storage paths.
	c = Default()
	c.Auth.Enabled = false
	c.Storage.Paths = nil
	if !hasProblem(c.Validate(), "at least one") {
		t.Error("expected empty-paths problem")
	}

	// Bad min_free.
	c = Default()
	c.Auth.Enabled = false
	c.Storage.Paths = []string{good}
	c.Storage.MinFree = "notasize"
	if !hasProblem(c.Validate(), "min_free") {
		t.Error("expected min_free problem")
	}

	// Bad log level.
	c = Default()
	c.Auth.Enabled = false
	c.Storage.Paths = []string{good}
	c.Log.Level = "loud"
	if !hasProblem(c.Validate(), "log.level") {
		t.Error("expected log.level problem")
	}

	// Bad log format.
	c = Default()
	c.Auth.Enabled = false
	c.Storage.Paths = []string{good}
	c.Log.Format = "xml"
	if !hasProblem(c.Validate(), "log.format") {
		t.Error("expected log.format problem")
	}
}

func TestDefaultLogValid(t *testing.T) {
	good := t.TempDir()
	c := Default()
	c.Auth.Enabled = false
	c.Storage.Paths = []string{good}
	if probs := c.Validate(); len(probs) != 0 {
		t.Fatalf("default log config should be valid, got %v", probs)
	}
	if c.Log.Level != "info" || c.Log.Format != "text" {
		t.Fatalf("unexpected log defaults: %+v", c.Log)
	}
}

func TestWriteTemplateForceOnExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := WriteTemplate(path, false); err != nil {
		t.Fatal(err)
	}
	// Second write without force must fail, with force must succeed.
	if err := WriteTemplate(path, false); err == nil {
		t.Fatal("expected refusal")
	}
	if err := WriteTemplate(path, true); err != nil {
		t.Fatalf("force: %v", err)
	}
}
