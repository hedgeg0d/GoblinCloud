package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFreeSpaceReal(t *testing.T) {
	// Exercise the real Statfs-backed implementation on a live directory.
	free, total := freeSpace(t.TempDir())
	if total == 0 {
		t.Fatal("expected non-zero total space on a real filesystem")
	}
	if free > total {
		t.Fatalf("free (%d) > total (%d)", free, total)
	}
}

func TestNewErrors(t *testing.T) {
	if _, err := New(nil, 0); err == nil {
		t.Fatal("expected error for no roots")
	}
	if _, err := New([]string{}, 0); err == nil {
		t.Fatal("expected error for empty roots")
	}
}

func TestRenameSourceMissing(t *testing.T) {
	s, _, _ := newTestStore(t, 1, 0)
	if err := s.Rename("/missing.txt", "/dst.txt"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestRenameInvalidPaths(t *testing.T) {
	s, _, _ := newTestStore(t, 1, 0)
	if err := s.Rename("/a\x00b", "/c"); err == nil {
		t.Fatal("expected error for NUL in source")
	}
	writeFile(t, s, "/ok.txt", "x")
	if err := s.Rename("/ok.txt", "/c\x00d"); err == nil {
		t.Fatal("expected error for NUL in target")
	}
}

func TestMkdirAndRemoveRootGuards(t *testing.T) {
	s, _, _ := newTestStore(t, 1, 0)
	if err := s.Mkdir("/"); err == nil {
		t.Fatal("expected error creating root")
	}
	if err := s.Remove("/"); err == nil {
		t.Fatal("expected error removing root")
	}
}

func TestCreateInvalidPath(t *testing.T) {
	s, _, _ := newTestStore(t, 1, 0)
	if _, err := s.OpenRead("/a\x00b"); err == nil {
		t.Fatal("expected error for NUL in OpenRead")
	}
	if _, err := s.Stat("/a\x00b"); err == nil {
		t.Fatal("expected error for NUL in Stat")
	}
	if _, err := s.List("/a\x00b"); err == nil {
		t.Fatal("expected error for NUL in List")
	}
	if _, err := s.ReadDir("/a\x00b"); err == nil {
		t.Fatal("expected error for NUL in ReadDir")
	}
	if err := s.Mkdir("/a\x00b"); err == nil {
		t.Fatal("expected error for NUL in Mkdir")
	}
	if err := s.Remove("/a\x00b"); err == nil {
		t.Fatal("expected error for NUL in Remove")
	}
}

func TestCopyFileMissingSource(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	if err := copyFile(filepath.Join(t.TempDir(), "nope"), dst); err == nil {
		t.Fatal("expected error copying missing source")
	}
}

func TestMoveTreeMissingSource(t *testing.T) {
	if err := moveTree(filepath.Join(t.TempDir(), "nope"), t.TempDir()); err == nil {
		t.Fatal("expected error moving missing source")
	}
}

func TestOpenReadMissing(t *testing.T) {
	s, _, _ := newTestStore(t, 1, 0)
	if _, err := s.OpenRead("/nope.txt"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestRenameCrossRootMove(t *testing.T) {
	// Two roots; file lives on root1. Rename keeps it reachable and moves the
	// logical path. Also exercises MkdirAll of the destination parent.
	s, roots, fake := newTestStore(t, 2, 0)
	fake.free[roots[0]] = 10 << 20
	fake.free[roots[1]] = 90 << 20
	writeFile(t, s, "/src.txt", "payload") // lands on root1 (emptiest)

	if err := s.Rename("/src.txt", "/nested/dst.txt"); err != nil {
		t.Fatalf("rename into new dir: %v", err)
	}
	f, err := s.OpenRead("/nested/dst.txt")
	if err != nil {
		t.Fatalf("open moved: %v", err)
	}
	f.Close()
	// Physically it stayed on root1 with the parent dir created there.
	if _, err := os.Stat(filepath.Join(roots[1], "nested", "dst.txt")); err != nil {
		t.Fatalf("expected file under root1/nested: %v", err)
	}
}
