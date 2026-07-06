package ftpsrv

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"goblin_cloud/internal/storage"
)

func newFS(t *testing.T) (*clientFS, []string) {
	t.Helper()
	roots := []string{t.TempDir(), t.TempDir()}
	store, err := storage.New(roots, 0)
	if err != nil {
		t.Fatal(err)
	}
	return &clientFS{store: store}, roots
}

func TestClientFSTransferRoundTrip(t *testing.T) {
	fs, roots := newFS(t)

	// Upload path (STOR): write through the balancer.
	h, err := fs.GetHandle("/up.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0)
	if err != nil {
		t.Fatalf("GetHandle write: %v", err)
	}
	if _, err := io.WriteString(h, "ftp-payload"); err != nil {
		t.Fatalf("write: %v", err)
	}
	h.Close()

	// The file must exist on exactly one physical root (balanced, not copied).
	placed := 0
	for _, r := range roots {
		if _, err := os.Stat(filepath.Join(r, "up.txt")); err == nil {
			placed++
		}
	}
	if placed != 1 {
		t.Fatalf("expected file on exactly one root, found on %d", placed)
	}

	// Stat via the adapter.
	info, err := fs.Stat("/up.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != int64(len("ftp-payload")) {
		t.Fatalf("size = %d", info.Size())
	}

	// Download path (RETR).
	rh, err := fs.GetHandle("/up.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("GetHandle read: %v", err)
	}
	b, _ := io.ReadAll(rh)
	rh.Close()
	if string(b) != "ftp-payload" {
		t.Fatalf("read back = %q", b)
	}
}

func TestClientFSListMkdirRenameRemove(t *testing.T) {
	fs, _ := newFS(t)

	if err := fs.MkdirAll("/dir", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// MkdirAll is idempotent.
	if err := fs.MkdirAll("/dir", 0o755); err != nil {
		t.Fatalf("MkdirAll idempotent: %v", err)
	}

	h, _ := fs.GetHandle("/dir/a.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0)
	io.WriteString(h, "x")
	h.Close()

	infos, err := fs.ReadDir("/dir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(infos) != 1 || infos[0].Name() != "a.txt" {
		t.Fatalf("ReadDir = %v", infos)
	}

	if err := fs.Rename("/dir/a.txt", "/dir/b.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := fs.Stat("/dir/a.txt"); err == nil {
		t.Fatal("old name should be gone")
	}
	if _, err := fs.Stat("/dir/b.txt"); err != nil {
		t.Fatalf("new name missing: %v", err)
	}

	if err := fs.RemoveAll("/dir"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := fs.ReadDir("/dir"); err == nil {
		t.Fatal("dir should be gone")
	}
}

func TestClientFSResumeOffset(t *testing.T) {
	fs, _ := newFS(t)
	h, _ := fs.GetHandle("/r.txt", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0)
	io.WriteString(h, "0123456789")
	h.Close()

	// Resume write at offset 5 (REST + STOR).
	h2, err := fs.GetHandle("/r.txt", os.O_WRONLY, 5)
	if err != nil {
		t.Fatalf("GetHandle offset: %v", err)
	}
	io.WriteString(h2, "XXXXX")
	h2.Close()

	rh, _ := fs.GetHandle("/r.txt", os.O_RDONLY, 0)
	b, _ := io.ReadAll(rh)
	rh.Close()
	if string(b) != "01234XXXXX" {
		t.Fatalf("resume write = %q", b)
	}
}

func TestIsWrite(t *testing.T) {
	writes := []int{os.O_WRONLY, os.O_RDWR, os.O_CREATE, os.O_APPEND, os.O_TRUNC, os.O_WRONLY | os.O_CREATE}
	for _, f := range writes {
		if !isWrite(f) {
			t.Errorf("isWrite(%d) = false, want true", f)
		}
	}
	if isWrite(os.O_RDONLY) {
		t.Error("isWrite(O_RDONLY) = true, want false")
	}
}
