package ftpsrv

import (
	"io"
	"os"
	"testing"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"
)

// TestClientFSAferoMethods exercises the plain afero.Fs surface (the fallback
// path ftpserverlib uses when the transfer/list extensions don't apply).
func TestClientFSAferoMethods(t *testing.T) {
	fs, _ := newFS(t)

	// Create + Open round trip.
	f, err := fs.Create("/a.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	io.WriteString(f, "hello")
	f.Close()

	rf, err := fs.Open("/a.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b, _ := io.ReadAll(rf)
	rf.Close()
	if string(b) != "hello" {
		t.Fatalf("read = %q", b)
	}

	// OpenFile for write then read.
	wf, err := fs.OpenFile("/b.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile write: %v", err)
	}
	io.WriteString(wf, "world")
	wf.Close()
	rf2, err := fs.OpenFile("/b.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile read: %v", err)
	}
	b2, _ := io.ReadAll(rf2)
	rf2.Close()
	if string(b2) != "world" {
		t.Fatalf("read = %q", b2)
	}

	// Mkdir + Remove of a file.
	if err := fs.Mkdir("/d", 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := fs.Remove("/a.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := fs.Stat("/a.txt"); err == nil {
		t.Fatal("file should be gone")
	}

	// Name + accepted no-op metadata ops.
	if fs.Name() == "" {
		t.Error("Name should be non-empty")
	}
	if err := fs.Chmod("/b.txt", 0o600); err != nil {
		t.Errorf("Chmod: %v", err)
	}
	if err := fs.Chown("/b.txt", 0, 0); err != nil {
		t.Errorf("Chown: %v", err)
	}
	if err := fs.Chtimes("/b.txt", time.Now(), time.Now()); err != nil {
		t.Errorf("Chtimes: %v", err)
	}
}

func TestDriverTrivialMethods(t *testing.T) {
	store, a := testDeps(t)
	d := &driver{store: store, auth: a, settings: &ftpserver.Settings{ListenAddr: ":0"}}

	if s, err := d.GetSettings(); err != nil || s == nil {
		t.Fatalf("GetSettings = %v, %v", s, err)
	}
	if msg, err := d.ClientConnected(nil); err != nil || msg == "" {
		t.Fatalf("ClientConnected = %q, %v", msg, err)
	}
	d.ClientDisconnected(nil) // must not panic
}
