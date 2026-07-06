package ftpsrv

import (
	"crypto/tls"
	"testing"

	"goblin_cloud/internal/auth"
	"goblin_cloud/internal/config"
	"goblin_cloud/internal/storage"
)

func testDeps(t *testing.T) (*storage.Store, *auth.Authenticator) {
	t.Helper()
	store, err := storage.New([]string{t.TempDir()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("pw")
	return store, auth.New(true, hash)
}

func baseCfg() config.Config {
	c := config.Default()
	c.FTP.Enabled = true
	c.FTP.Listen = ":0"
	c.FTP.PassivePorts = "30000-30010"
	return c
}

func TestNewValid(t *testing.T) {
	store, a := testDeps(t)
	srv, err := New(store, a, baseCfg(), nil)
	if err != nil || srv == nil {
		t.Fatalf("New = %v, %v", srv, err)
	}
}

func TestNewBadPassiveRange(t *testing.T) {
	store, a := testDeps(t)
	c := baseCfg()
	c.FTP.PassivePorts = "not-a-range"
	if _, err := New(store, a, c, nil); err == nil {
		t.Fatal("expected error for bad passive_ports")
	}
}

func TestNewTLSWithoutCert(t *testing.T) {
	store, a := testDeps(t)
	c := baseCfg()
	c.FTP.TLS = true
	if _, err := New(store, a, c, nil); err == nil {
		t.Fatal("expected error: ftp.tls enabled but no certificate")
	}
}

func TestDriverAuthUser(t *testing.T) {
	store, a := testDeps(t)
	d := &driver{store: store, auth: a}

	if _, err := d.AuthUser(nil, "user", "pw"); err != nil {
		t.Errorf("correct password rejected: %v", err)
	}
	if _, err := d.AuthUser(nil, "user", "nope"); err == nil {
		t.Error("wrong password accepted")
	}
}

func TestDriverGetTLSConfig(t *testing.T) {
	d := &driver{}
	if _, err := d.GetTLSConfig(); err == nil {
		t.Error("expected error when TLS not configured")
	}
	d.tlsConfig = &tls.Config{}
	if _, err := d.GetTLSConfig(); err != nil {
		t.Errorf("expected TLS config, got %v", err)
	}
}
