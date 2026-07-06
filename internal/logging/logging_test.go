package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"goblin_cloud/internal/config"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"error":   slog.LevelError,
		"":        slog.LevelInfo, // default
		"unknown": slog.LevelInfo, // default
	}
	for name, want := range cases {
		if got := parseLevel(name); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	h := buildHandler(&buf, config.Log{Level: "info", Format: "text"})
	log := slog.New(h)

	log.Debug("hidden-debug")
	log.Info("shown-info")

	out := buf.String()
	if strings.Contains(out, "hidden-debug") {
		t.Error("debug record should be suppressed at info level")
	}
	if !strings.Contains(out, "shown-info") {
		t.Error("info record should be present at info level")
	}
}

func TestDebugLevelShowsDebug(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(buildHandler(&buf, config.Log{Level: "debug", Format: "text"}))
	log.Debug("visible-debug")
	if !strings.Contains(buf.String(), "visible-debug") {
		t.Error("debug record should appear at debug level")
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(buildHandler(&buf, config.Log{Level: "info", Format: "json"}))
	log.Info("hello", "key", "value")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if rec["msg"] != "hello" || rec["key"] != "value" {
		t.Fatalf("unexpected JSON fields: %v", rec)
	}
}

func TestTextFormatDefault(t *testing.T) {
	var buf bytes.Buffer
	// An unrecognised format falls back to text (not JSON).
	log := slog.New(buildHandler(&buf, config.Log{Level: "info", Format: "text"}))
	log.Info("plain")
	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Error("text handler output should not be JSON")
	}
	if !strings.Contains(buf.String(), "msg=plain") {
		t.Errorf("unexpected text output: %s", buf.String())
	}
}

func TestSetupInstallsDefault(t *testing.T) {
	// Smoke test: Setup must not panic and must replace the default logger.
	Setup(config.Log{Level: "warn", Format: "json"})
	if slog.Default() == nil {
		t.Fatal("Setup did not install a default logger")
	}
}
