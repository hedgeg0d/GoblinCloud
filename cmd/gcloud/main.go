// Command gcloud is the Goblin Cloud server and admin tool.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"

	"goblin_cloud/internal/auth"
	"goblin_cloud/internal/config"
	"goblin_cloud/internal/logging"
	"goblin_cloud/internal/server"
	"goblin_cloud/internal/storage"
)

// version is the single source of truth for the release version. Release builds
// may override it via -ldflags "-X main.version=..." (e.g. to stamp a git tag).
var version = "0.1.0"

const usage = `Goblin Cloud — FTP, REST API and web UI for your files.

Usage:
  gcloud [--config <path>] <command>

Commands:
  serve            Start all enabled services (default)
  set password     Set the access password (interactive)
  config init      Write a starter config file
  config path      Print the resolved config path
  config check     Validate the config without starting
  storage status   Show per-root free space and balance
  version          Print version information
`

func main() {
	cfgPath, args := extractConfigFlag(os.Args[1:])

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	var err error
	switch cmd {
	case "serve":
		err = cmdServe(cfgPath)
	case "set":
		err = cmdSet(cfgPath, args)
	case "config":
		err = cmdConfig(cfgPath, args)
	case "storage":
		err = cmdStorage(cfgPath, args)
	case "version", "--version", "-v":
		fmt.Printf("gcloud %s\n", version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCode(err))
	}
}

// extractConfigFlag pulls a --config/-c value out of args, wherever it sits.
func extractConfigFlag(in []string) (cfgPath string, rest []string) {
	for i := 0; i < len(in); i++ {
		a := in[i]
		switch {
		case a == "--config" || a == "-c":
			if i+1 < len(in) {
				cfgPath = in[i+1]
				i++
			}
		case strings.HasPrefix(a, "--config="):
			cfgPath = strings.TrimPrefix(a, "--config=")
		default:
			rest = append(rest, a)
		}
	}
	return cfgPath, rest
}

func cmdServe(cfgPath string) error {
	path := config.Resolve(cfgPath)
	cfg, err := config.Load(path)
	if err != nil {
		return &codedError{2, err}
	}
	if probs := cfg.Validate(); len(probs) > 0 {
		return &codedError{2, fmt.Errorf("invalid config:\n  %s", strings.Join(probs, "\n  "))}
	}
	logging.Setup(cfg.Log)
	slog.Info("starting goblin cloud", "config", path, "version", version)
	if err := server.Run(cfg, version); err != nil {
		return &codedError{3, err}
	}
	return nil
}

func cmdSet(cfgPath string, args []string) error {
	if len(args) < 1 || args[0] != "password" {
		return errors.New("usage: gcloud set password")
	}
	path := config.Resolve(cfgPath)

	pw, err := promptPassword("New password: ")
	if err != nil {
		return err
	}
	if len(pw) == 0 {
		return errors.New("password cannot be empty")
	}
	confirm, err := promptPassword("Confirm password: ")
	if err != nil {
		return err
	}
	if pw != confirm {
		return errors.New("passwords do not match")
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if err := config.SetPasswordHash(path, hash); err != nil {
		return err
	}
	fmt.Printf("Password updated in %s\n", path)
	return nil
}

func cmdConfig(cfgPath string, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: gcloud config <init|path|check>")
	}
	path := config.Resolve(cfgPath)
	switch args[0] {
	case "init":
		force := contains(args[1:], "--force")
		if err := config.WriteTemplate(path, force); err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", path)
		return nil
	case "path":
		fmt.Println(path)
		return nil
	case "check":
		cfg, err := config.Load(path)
		if err != nil {
			return &codedError{2, err}
		}
		probs := cfg.Validate()
		if len(probs) == 0 {
			fmt.Printf("%s is valid\n", path)
			return nil
		}
		for _, p := range probs {
			fmt.Fprintln(os.Stderr, "-", p)
		}
		return &codedError{2, fmt.Errorf("%d problem(s) found", len(probs))}
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func cmdStorage(cfgPath string, args []string) error {
	if len(args) < 1 || args[0] != "status" {
		return errors.New("usage: gcloud storage status")
	}
	path := config.Resolve(cfgPath)
	cfg, err := config.Load(path)
	if err != nil {
		return &codedError{2, err}
	}
	minFree, err := cfg.MinFreeBytes()
	if err != nil {
		return err
	}
	store, err := storage.New(cfg.Storage.Paths, minFree)
	if err != nil {
		return err
	}
	fmt.Printf("%-30s %10s %10s  %s\n", "ROOT", "TOTAL", "FREE", "WRITABLE")
	for _, st := range store.Status() {
		writable := "no"
		if st.Writable {
			writable = "yes"
		}
		fmt.Printf("%-30s %10s %10s  %s\n", st.Path, humanBytes(st.Total), humanBytes(st.Free), writable)
	}
	return nil
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

func exitCode(err error) int {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return 1
}
