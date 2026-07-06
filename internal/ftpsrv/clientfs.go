package ftpsrv

import (
	"io/fs"
	"os"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"

	"goblin_cloud/internal/storage"
)

// clientFS adapts *storage.Store to the afero.Fs surface FTP expects. Directory
// listing and file transfers go through the ftpserverlib extensions below so we
// never need a custom afero.File — *os.File already satisfies afero.File.
type clientFS struct {
	store *storage.Store
}

var (
	_ afero.Fs                                      = (*clientFS)(nil)
	_ ftpserver.ClientDriverExtensionFileList       = (*clientFS)(nil)
	_ ftpserver.ClientDriverExtentionFileTransfer   = (*clientFS)(nil)
)

func (c *clientFS) Name() string { return "goblincloud" }

func (c *clientFS) Create(name string) (afero.File, error) {
	return c.store.Create(name)
}

func (c *clientFS) Open(name string) (afero.File, error) {
	return c.store.OpenRead(name)
}

func (c *clientFS) OpenFile(name string, flag int, _ os.FileMode) (afero.File, error) {
	if isWrite(flag) {
		return c.store.OpenWrite(name, flag, 0)
	}
	return c.store.OpenRead(name)
}

func (c *clientFS) Mkdir(name string, _ os.FileMode) error {
	return c.store.Mkdir(name)
}

func (c *clientFS) MkdirAll(path string, _ os.FileMode) error {
	if err := c.store.Mkdir(path); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}

func (c *clientFS) Remove(name string) error    { return c.store.Remove(name) }
func (c *clientFS) RemoveAll(path string) error { return c.store.Remove(path) }

func (c *clientFS) Rename(oldname, newname string) error {
	return c.store.Rename(oldname, newname)
}

func (c *clientFS) Stat(name string) (os.FileInfo, error) {
	return c.store.Stat(name)
}

// The FTP server does not need these; keep them as accepted no-ops.
func (c *clientFS) Chmod(string, os.FileMode) error        { return nil }
func (c *clientFS) Chown(string, int, int) error           { return nil }
func (c *clientFS) Chtimes(string, time.Time, time.Time) error { return nil }

// ReadDir satisfies ClientDriverExtensionFileList (merged listing).
func (c *clientFS) ReadDir(name string) ([]fs.FileInfo, error) {
	return c.store.ReadDir(name)
}

// GetHandle satisfies ClientDriverExtentionFileTransfer, routing writes through
// the balancer and honouring resume offsets.
func (c *clientFS) GetHandle(name string, flags int, offset int64) (ftpserver.FileTransfer, error) {
	if isWrite(flags) {
		return c.store.OpenWrite(name, flags, offset)
	}
	f, err := c.store.OpenRead(name)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func isWrite(flag int) bool {
	return flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC) != 0
}
