// Package storage implements the merged, DB-free file layer: a single logical
// tree spread across several physical roots, balanced by free space.
package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// ErrNoSpace is returned when no root has room for a write.
var ErrNoSpace = errors.New("no storage root has enough free space")

// ErrNotExist mirrors os.ErrNotExist for callers that don't import os.
var ErrNotExist = os.ErrNotExist

// Store is the merged view over one or more physical roots.
type Store struct {
	roots   []string
	minFree uint64
}

// Entry is one item in a directory listing.
type Entry struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"-"`
	Type    string    `json:"type"` // "file" or "dir"
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modified"`
}

// RootStatus reports the state of one physical root.
type RootStatus struct {
	Path     string
	Total    uint64
	Free     uint64
	Writable bool
}

// New builds a Store. Paths are made absolute; minFree is the write margin.
func New(paths []string, minFree uint64) (*Store, error) {
	if len(paths) == 0 {
		return nil, errors.New("storage: no roots configured")
	}
	roots := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("storage: %w", err)
		}
		roots = append(roots, filepath.Clean(abs))
	}
	return &Store{roots: roots, minFree: minFree}, nil
}

// clean normalises a logical path to a rooted, traversal-safe slash path.
func clean(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", errors.New("invalid path")
	}
	c := path.Clean("/" + strings.TrimPrefix(p, "/"))
	return c, nil
}

// phys joins a logical path onto a physical root.
func phys(root, logical string) string {
	return filepath.Join(root, filepath.FromSlash(logical))
}

// findRoot returns the first root that contains logical.
func (s *Store) findRoot(logical string) (string, bool) {
	for _, r := range s.roots {
		if _, err := os.Lstat(phys(r, logical)); err == nil {
			return r, true
		}
	}
	return "", false
}

// freeSpace returns available bytes on the filesystem holding dir.
func freeSpace(dir string) (free, total uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}

// chooseWriteRoot picks the eligible root with the most free space.
func (s *Store) chooseWriteRoot() (string, error) {
	best := ""
	var bestFree uint64
	for _, r := range s.roots {
		free, _ := freeSpace(r)
		if free < s.minFree {
			continue
		}
		if best == "" || free > bestFree {
			best, bestFree = r, free
		}
	}
	if best == "" {
		return "", ErrNoSpace
	}
	return best, nil
}

// merged returns the union of directory entries across all roots, first root
// winning on name collisions. found reports whether the dir existed anywhere.
func (s *Store) merged(lp string) (infos []fs.FileInfo, found bool) {
	seen := map[string]fs.FileInfo{}
	order := []string{}
	for _, r := range s.roots {
		items, err := os.ReadDir(phys(r, lp))
		if err != nil {
			continue // dir may not exist on this root
		}
		found = true
		for _, it := range items {
			if _, ok := seen[it.Name()]; ok {
				continue
			}
			info, err := it.Info()
			if err != nil {
				continue
			}
			seen[it.Name()] = info
			order = append(order, it.Name())
		}
	}
	for _, name := range order {
		infos = append(infos, seen[name])
	}
	return infos, found
}

// ReadDir returns the merged directory listing as fs.FileInfo (used by FTP).
func (s *Store) ReadDir(logical string) ([]fs.FileInfo, error) {
	lp, err := clean(logical)
	if err != nil {
		return nil, err
	}
	infos, found := s.merged(lp)
	if !found {
		return nil, fmt.Errorf("readdir %s: %w", lp, ErrNotExist)
	}
	return infos, nil
}

// List returns the union of entries across all roots, sorted dirs-first then by
// name, first root winning on collisions.
func (s *Store) List(logical string) ([]Entry, error) {
	lp, err := clean(logical)
	if err != nil {
		return nil, err
	}
	infos, found := s.merged(lp)
	if !found {
		return nil, fmt.Errorf("list %s: %w", lp, ErrNotExist)
	}
	out := make([]Entry, 0, len(infos))
	for _, info := range infos {
		out = append(out, entryFromInfo(info))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir // dirs first
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func entryFromInfo(info fs.FileInfo) Entry {
	t := "file"
	if info.IsDir() {
		t = "dir"
	}
	return Entry{
		Name:    info.Name(),
		IsDir:   info.IsDir(),
		Type:    t,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
}

// Stat returns file info for a logical path.
func (s *Store) Stat(logical string) (fs.FileInfo, error) {
	lp, err := clean(logical)
	if err != nil {
		return nil, err
	}
	r, ok := s.findRoot(lp)
	if !ok {
		return nil, fmt.Errorf("stat %s: %w", lp, ErrNotExist)
	}
	return os.Stat(phys(r, lp))
}

// OpenRead opens a file for reading from the first root that has it.
func (s *Store) OpenRead(logical string) (*os.File, error) {
	lp, err := clean(logical)
	if err != nil {
		return nil, err
	}
	r, ok := s.findRoot(lp)
	if !ok {
		return nil, fmt.Errorf("open %s: %w", lp, ErrNotExist)
	}
	return os.Open(phys(r, lp))
}

// writeRootFor returns the root a write should target: the root already holding
// the file, or otherwise the emptiest eligible root.
func (s *Store) writeRootFor(lp string) (string, error) {
	if r, ok := s.findRoot(lp); ok {
		return r, nil
	}
	return s.chooseWriteRoot()
}

// Create opens a file for writing (truncating), choosing the balanced root and
// creating the parent directory there as needed.
func (s *Store) Create(logical string) (*os.File, error) {
	return s.OpenWrite(logical, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0)
}

// OpenWrite opens a file for writing with explicit flags (used by FTP for
// append/resume). offset seeks the returned file when > 0.
func (s *Store) OpenWrite(logical string, flags int, offset int64) (*os.File, error) {
	lp, err := clean(logical)
	if err != nil {
		return nil, err
	}
	root, err := s.writeRootFor(lp)
	if err != nil {
		return nil, err
	}
	target := phys(root, lp)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(target, flags, 0o644)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// Mkdir creates a directory on every root so listings stay consistent.
func (s *Store) Mkdir(logical string) error {
	lp, err := clean(logical)
	if err != nil {
		return err
	}
	if lp == "/" {
		return errors.New("cannot create root")
	}
	if _, ok := s.findRoot(lp); ok {
		return fmt.Errorf("mkdir %s: %w", lp, os.ErrExist)
	}
	var firstErr error
	for _, r := range s.roots {
		if err := os.MkdirAll(phys(r, lp), 0o755); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Remove deletes a file or directory (recursively) from every root that has it.
func (s *Store) Remove(logical string) error {
	lp, err := clean(logical)
	if err != nil {
		return err
	}
	if lp == "/" {
		return errors.New("cannot remove root")
	}
	removed := false
	var firstErr error
	for _, r := range s.roots {
		p := phys(r, lp)
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed = true
	}
	if firstErr != nil {
		return firstErr
	}
	if !removed {
		return fmt.Errorf("remove %s: %w", lp, ErrNotExist)
	}
	return nil
}

// Rename moves a file or directory. Within one root it is a cheap rename;
// across roots it is a copy-then-delete move.
func (s *Store) Rename(from, to string) error {
	fp, err := clean(from)
	if err != nil {
		return err
	}
	tp, err := clean(to)
	if err != nil {
		return err
	}
	srcRoot, ok := s.findRoot(fp)
	if !ok {
		return fmt.Errorf("rename %s: %w", fp, ErrNotExist)
	}
	if _, ok := s.findRoot(tp); ok {
		return fmt.Errorf("rename to %s: %w", tp, os.ErrExist)
	}

	src := phys(srcRoot, fp)
	dst := phys(srcRoot, tp)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Fall back to a cross-device move.
	if err := moveTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// moveTree copies a file or directory tree from src to dst.
func moveTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		items, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, it := range items {
			if err := moveTree(filepath.Join(src, it.Name()), filepath.Join(dst, it.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Status reports per-root usage and write-eligibility.
func (s *Store) Status() []RootStatus {
	out := make([]RootStatus, 0, len(s.roots))
	for _, r := range s.roots {
		free, total := freeSpace(r)
		out = append(out, RootStatus{
			Path:     r,
			Total:    total,
			Free:     free,
			Writable: free >= s.minFree,
		})
	}
	return out
}
