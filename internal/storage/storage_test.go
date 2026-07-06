package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestStore builds a Store over freshly created temp roots and lets the test
// control reported free space via the returned fake.
func newTestStore(t *testing.T, nroots int, minFree uint64) (*Store, []string, *fakeSpace) {
	t.Helper()
	roots := make([]string, nroots)
	for i := range roots {
		roots[i] = t.TempDir()
	}
	s, err := New(roots, minFree)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fake := &fakeSpace{free: map[string]uint64{}}
	for _, r := range roots {
		fake.free[r] = 100 << 20 // 100 MiB default
	}
	s.stat = fake.stat
	return s, roots, fake
}

type fakeSpace struct {
	free map[string]uint64
}

func (f *fakeSpace) stat(root string) (free, total uint64) {
	return f.free[root], 200 << 20
}

func writeFile(t *testing.T, s *Store, logical, content string) {
	t.Helper()
	f, err := s.Create(logical)
	if err != nil {
		t.Fatalf("Create %s: %v", logical, err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		t.Fatalf("write %s: %v", logical, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", logical, err)
	}
}

// existsIn reports which root physically holds the logical path.
func existsIn(roots []string, logical string) []string {
	var hits []string
	for _, r := range roots {
		if _, err := os.Lstat(filepath.Join(r, filepath.FromSlash(logical))); err == nil {
			hits = append(hits, r)
		}
	}
	return hits
}

func TestCleanRejectsAndContainsTraversal(t *testing.T) {
	s, roots, _ := newTestStore(t, 1, 0)

	// A traversal attempt must resolve back inside a root, never escape it.
	writeFile(t, s, "/../../etc/evil.txt", "x")
	escaped := filepath.Join(filepath.Dir(roots[0]), "etc", "evil.txt")
	if _, err := os.Stat(escaped); err == nil {
		t.Fatalf("path traversal escaped the root: %s", escaped)
	}
	if hits := existsIn(roots, "/etc/evil.txt"); len(hits) == 0 {
		t.Fatal("expected traversal to be confined to /etc/evil.txt inside root")
	}

	// A NUL byte is rejected outright.
	if _, err := s.Create("/a\x00b"); err == nil {
		t.Fatal("expected error for NUL byte in path")
	}
}

func TestBalancingPicksEmptiestRoot(t *testing.T) {
	s, roots, fake := newTestStore(t, 3, 0)
	fake.free[roots[0]] = 10 << 20
	fake.free[roots[1]] = 90 << 20 // emptiest
	fake.free[roots[2]] = 50 << 20

	writeFile(t, s, "/file.bin", "data")

	hits := existsIn(roots, "/file.bin")
	if len(hits) != 1 || hits[0] != roots[1] {
		t.Fatalf("expected file on emptiest root %s, got %v", roots[1], hits)
	}
}

func TestBalancingSkipsRootsBelowMinFree(t *testing.T) {
	minFree := uint64(20 << 20)
	s, roots, fake := newTestStore(t, 2, minFree)
	fake.free[roots[0]] = 100 << 20 // has most free, but...
	fake.free[roots[1]] = 5 << 20   // below min_free -> skipped

	writeFile(t, s, "/a.bin", "x")
	if hits := existsIn(roots, "/a.bin"); len(hits) != 1 || hits[0] != roots[0] {
		t.Fatalf("expected write on root0, got %v", hits)
	}

	// Now flip: only root1 eligible.
	fake.free[roots[0]] = 1 << 20
	fake.free[roots[1]] = 100 << 20
	writeFile(t, s, "/b.bin", "x")
	if hits := existsIn(roots, "/b.bin"); len(hits) != 1 || hits[0] != roots[1] {
		t.Fatalf("expected write on root1, got %v", hits)
	}
}

func TestBalancingNoSpace(t *testing.T) {
	minFree := uint64(50 << 20)
	s, roots, fake := newTestStore(t, 2, minFree)
	for _, r := range roots {
		fake.free[r] = 1 << 20 // everyone below min_free
	}
	if _, err := s.Create("/nope.bin"); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("expected ErrNoSpace, got %v", err)
	}
}

// TestBalancingSpreadsAcrossDisks simulates real disks whose free space shrinks
// as data is written: the emptiest-root rule should spread files over both.
func TestBalancingSpreadsAcrossDisks(t *testing.T) {
	s, roots, fake := newTestStore(t, 2, 0)
	const cap = 100 << 20
	// Free space tracks bytes actually written under each root.
	fake.free = map[string]uint64{}
	dynamic := func(root string) (uint64, uint64) {
		used := dirSize(root)
		return cap - uint64(used), cap
	}
	s.stat = dynamic

	const chunk = 4 << 20 // 4 MiB per file
	payload := strings.Repeat("x", chunk)
	for i := 0; i < 10; i++ {
		writeFile(t, s, "/f"+string(rune('0'+i))+".bin", payload)
	}

	u0, u1 := dirSize(roots[0]), dirSize(roots[1])
	if u0 == 0 || u1 == 0 {
		t.Fatalf("expected files spread across both roots, got root0=%d root1=%d", u0, u1)
	}
	// Balanced within one chunk of each other.
	diff := u0 - u1
	if diff < 0 {
		diff = -diff
	}
	if diff > chunk {
		t.Fatalf("distribution not even: root0=%d root1=%d (diff=%d)", u0, u1, diff)
	}
}

func TestOverwriteStaysOnSameRoot(t *testing.T) {
	s, roots, fake := newTestStore(t, 2, 0)
	fake.free[roots[0]] = 90 << 20
	fake.free[roots[1]] = 10 << 20
	writeFile(t, s, "/x.bin", "first") // -> root0

	// Flip free space so the balancer would now prefer root1.
	fake.free[roots[0]] = 10 << 20
	fake.free[roots[1]] = 90 << 20
	writeFile(t, s, "/x.bin", "second") // must overwrite on root0, not copy to root1

	hits := existsIn(roots, "/x.bin")
	if len(hits) != 1 || hits[0] != roots[0] {
		t.Fatalf("overwrite should stay on root0, got %v", hits)
	}
	f, _ := s.OpenRead("/x.bin")
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "second" {
		t.Fatalf("expected overwritten content, got %q", b)
	}
}

func TestMergedListUnionAndDedup(t *testing.T) {
	s, roots, fake := newTestStore(t, 2, 0)
	// Force placement by toggling free space per write.
	fake.free[roots[0]] = 90 << 20
	fake.free[roots[1]] = 10 << 20
	writeFile(t, s, "/shared.txt", "root0-wins")
	writeFile(t, s, "/only0.txt", "x")

	fake.free[roots[0]] = 10 << 20
	fake.free[roots[1]] = 90 << 20
	writeFile(t, s, "/only1.txt", "y")
	// Also create a colliding name on root1 directly.
	os.WriteFile(filepath.Join(roots[1], "shared.txt"), []byte("root1-loses"), 0o644)

	if err := s.Mkdir("/adir"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	entries, err := s.List("/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	names := map[string]Entry{}
	for _, e := range entries {
		names[e.Name] = e
	}
	for _, want := range []string{"shared.txt", "only0.txt", "only1.txt", "adir"} {
		if _, ok := names[want]; !ok {
			t.Fatalf("missing %q in merged listing: %v", want, entries)
		}
	}
	// Dedup: shared.txt appears once and the first root wins.
	count := 0
	for _, e := range entries {
		if e.Name == "shared.txt" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("shared.txt should appear once, got %d", count)
	}
	// Directories sort before files.
	if entries[0].Type != "dir" {
		t.Fatalf("expected dir first, got %+v", entries[0])
	}
}

func TestMkdirOnAllRootsAndExist(t *testing.T) {
	s, roots, _ := newTestStore(t, 3, 0)
	if err := s.Mkdir("/photos"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, r := range roots {
		if info, err := os.Stat(filepath.Join(r, "photos")); err != nil || !info.IsDir() {
			t.Fatalf("expected /photos on %s", r)
		}
	}
	if err := s.Mkdir("/photos"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected ErrExist, got %v", err)
	}
}

func TestRemoveFromAllRoots(t *testing.T) {
	s, roots, _ := newTestStore(t, 2, 0)
	// Same-named dir exists on both roots (mkdir spreads it), plus files each.
	s.Mkdir("/d")
	os.WriteFile(filepath.Join(roots[0], "d", "a"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(roots[1], "d", "b"), []byte("2"), 0o644)

	if err := s.Remove("/d"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	for _, r := range roots {
		if _, err := os.Stat(filepath.Join(r, "d")); !os.IsNotExist(err) {
			t.Fatalf("expected /d gone from %s", r)
		}
	}
	if err := s.Remove("/missing"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestRenameFileAndDir(t *testing.T) {
	s, _, _ := newTestStore(t, 2, 0)
	writeFile(t, s, "/old.txt", "hello")
	if err := s.Rename("/old.txt", "/new.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := s.Stat("/old.txt"); !errors.Is(err, ErrNotExist) {
		t.Fatal("old path should be gone")
	}
	f, err := s.OpenRead("/new.txt")
	if err != nil {
		t.Fatalf("open new: %v", err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "hello" {
		t.Fatalf("content lost on rename: %q", b)
	}

	// Rename onto an existing target must fail.
	writeFile(t, s, "/taken.txt", "x")
	if err := s.Rename("/new.txt", "/taken.txt"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected ErrExist, got %v", err)
	}
}

func TestReadDirAndStatErrors(t *testing.T) {
	s, _, _ := newTestStore(t, 1, 0)
	writeFile(t, s, "/dir/a.txt", "x")

	infos, err := s.ReadDir("/dir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(infos) != 1 || infos[0].Name() != "a.txt" {
		t.Fatalf("unexpected ReadDir result: %v", infos)
	}
	if _, err := s.Stat("/nope"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
	if _, err := s.List("/nope"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("expected ErrNotExist for List, got %v", err)
	}
}

func TestStatusReportsWritable(t *testing.T) {
	s, roots, fake := newTestStore(t, 2, 50<<20)
	fake.free[roots[0]] = 100 << 20
	fake.free[roots[1]] = 10 << 20

	st := s.Status()
	if len(st) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(st))
	}
	if !st[0].Writable {
		t.Fatal("root0 should be writable")
	}
	if st[1].Writable {
		t.Fatal("root1 should not be writable")
	}
}

func TestMoveTreeAcrossDirs(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "moved")
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "f.txt"), []byte("deep"), 0o644)

	if err := moveTree(src, dst); err != nil {
		t.Fatalf("moveTree: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "sub", "f.txt"))
	if err != nil || string(b) != "deep" {
		t.Fatalf("moveTree lost data: %v %q", err, b)
	}
}

// dirSize returns the total bytes of regular files under root.
func dirSize(root string) int64 {
	var total int64
	filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
