package server

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"goblin_cloud/internal/config"
)

// freePort returns a currently-free TCP port on the loopback interface.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func lanConfig(t *testing.T) config.Config {
	c := config.Default()
	c.Auth.Enabled = false // no password needed for the test
	c.Storage.Paths = []string{t.TempDir()}
	c.Storage.MinFree = "0"
	c.Server.Listen = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	c.FTP.Enabled = true
	c.FTP.Listen = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	c.FTP.PassivePorts = "40000-40010"
	return c
}

func TestRunStartsAndShutsDown(t *testing.T) {
	cfg := lanConfig(t)

	done := make(chan error, 1)
	go func() { done <- Run(cfg) }()

	// Wait for the HTTP server to come up.
	base := "http://" + cfg.Server.Listen + "/api/files?path=/"
	up := false
	for i := 0; i < 100; i++ {
		res, err := http.Get(base)
		if err == nil {
			res.Body.Close()
			up = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !up {
		t.Fatal("http server did not come up")
	}

	// Give Run a beat to reach its signal select, then ask it to stop.
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error on clean shutdown: %v", err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("Run did not shut down in time")
	}
}

func TestRunBadMinFree(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Enabled = false
	cfg.Storage.Paths = []string{t.TempDir()}
	cfg.Storage.MinFree = "not-a-size"
	if err := Run(cfg); err == nil {
		t.Fatal("expected error for bad min_free")
	}
}

func TestRunNoStorageRoots(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Enabled = false
	cfg.Storage.Paths = nil
	cfg.Storage.MinFree = "0"
	if err := Run(cfg); err == nil {
		t.Fatal("expected error for missing storage roots")
	}
}

func TestRunBadPassivePorts(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Enabled = false
	cfg.Storage.Paths = []string{t.TempDir()}
	cfg.Storage.MinFree = "0"
	cfg.Server.Listen = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cfg.FTP.Enabled = true
	cfg.FTP.PassivePorts = "bogus"
	if err := Run(cfg); err == nil {
		t.Fatal("expected error for bad passive_ports")
	}
}
