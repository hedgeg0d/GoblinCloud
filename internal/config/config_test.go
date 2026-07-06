package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	cases := map[string]uint64{
		"":       0,
		"1024":   1024,
		"1KB":    1 << 10,
		"1 kb":   1 << 10,
		"512MB":  512 << 20,
		"2GB":    2 << 30,
		"1TB":    1 << 40,
		"1.5GB":  uint64(1.5 * float64(1<<30)),
		"1G":     1 << 30,
		"10 MiB": 0, // "MiB" not a recognised suffix -> falls through, "10 MIB" parse fails
	}
	for in, want := range cases {
		got, err := ParseSize(in)
		if in == "10 MiB" {
			if err == nil {
				t.Errorf("ParseSize(%q) expected error", in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSize(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestPassivePortRange(t *testing.T) {
	c := Default()
	c.FTP.PassivePorts = "30000-30100"
	lo, hi, err := c.PassivePortRange()
	if err != nil || lo != 30000 || hi != 30100 {
		t.Fatalf("got %d-%d err=%v", lo, hi, err)
	}
	for _, bad := range []string{"", "abc", "100", "500-100", "-1-5"} {
		c.FTP.PassivePorts = bad
		if _, _, err := c.PassivePortRange(); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestLoadMergesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`
[server]
listen = ":9999"
[storage]
paths = ["/tmp/x"]
`), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != ":9999" {
		t.Errorf("listen not read: %q", cfg.Server.Listen)
	}
	// Unspecified keys keep their defaults.
	if cfg.FTP.Listen != ":2121" {
		t.Errorf("ftp.listen default lost: %q", cfg.FTP.Listen)
	}
	if cfg.Storage.MinFree != "1GB" {
		t.Errorf("min_free default lost: %q", cfg.Storage.MinFree)
	}
	if cfg.Path() != path {
		t.Errorf("Path() = %q", cfg.Path())
	}
}

func TestValidate(t *testing.T) {
	good := t.TempDir()
	base := Default()
	base.Auth.Enabled = false
	base.Storage.Paths = []string{good}

	if probs := base.Validate(); len(probs) != 0 {
		t.Fatalf("expected valid, got %v", probs)
	}

	// Missing password when auth enabled.
	c := base
	c.Auth.Enabled = true
	c.Auth.PasswordHash = ""
	if !hasProblem(c.Validate(), "password_hash") {
		t.Error("expected password_hash problem")
	}

	// Domain set but no email.
	c = base
	c.Server.Domain = "files.example.com"
	c.Server.AutocertEmail = ""
	if !hasProblem(c.Validate(), "autocert_email") {
		t.Error("expected autocert_email problem")
	}

	// Nonexistent storage path.
	c = base
	c.Storage.Paths = []string{filepath.Join(good, "does-not-exist")}
	if !hasProblem(c.Validate(), "does not exist") {
		t.Error("expected missing-path problem")
	}

	// Bad passive range with FTP enabled.
	c = base
	c.FTP.Enabled = true
	c.FTP.PassivePorts = "nope"
	if !hasProblem(c.Validate(), "passive_ports") {
		t.Error("expected passive_ports problem")
	}
}

func TestWriteTemplateAndNoOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := WriteTemplate(path, false); err != nil {
		t.Fatalf("WriteTemplate: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("template not written: %v", err)
	}
	// Refuses to clobber.
	if err := WriteTemplate(path, false); err == nil {
		t.Fatal("expected refusal to overwrite")
	}
	// Force overwrites.
	if err := WriteTemplate(path, true); err != nil {
		t.Fatalf("force overwrite failed: %v", err)
	}
	// The written template must load and validate structurally.
	if _, err := Load(path); err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
}

func TestSetPasswordHashPreservesComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteTemplate(path, false); err != nil {
		t.Fatal(err)
	}
	if err := SetPasswordHash(path, "$2a$10$abcdef"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, `password_hash = "$2a$10$abcdef"`) {
		t.Fatalf("hash not written:\n%s", s)
	}
	// Comments from the template survive.
	if !strings.Contains(s, "# Goblin Cloud configuration.") {
		t.Fatal("comments were lost")
	}
	// Exactly one password_hash line.
	if n := strings.Count(s, "password_hash ="); n != 1 {
		t.Fatalf("expected 1 password_hash line, got %d", n)
	}

	// Loading it back yields the hash.
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.PasswordHash != "$2a$10$abcdef" {
		t.Fatalf("round-trip hash mismatch: %q", cfg.Auth.PasswordHash)
	}
}

func TestSetPasswordHashCreatesWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := SetPasswordHash(path, "$2a$10$xyz"); err != nil {
		t.Fatalf("SetPasswordHash on missing file: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.PasswordHash != "$2a$10$xyz" {
		t.Fatalf("hash mismatch: %q", cfg.Auth.PasswordHash)
	}
}

func TestGlobalMode(t *testing.T) {
	c := Default()
	if c.Global() {
		t.Error("empty domain should be LAN mode")
	}
	c.Server.Domain = "x.com"
	if !c.Global() {
		t.Error("set domain should be global mode")
	}
}

func hasProblem(probs []string, substr string) bool {
	for _, p := range probs {
		if strings.Contains(p, substr) {
			return true
		}
	}
	return false
}
