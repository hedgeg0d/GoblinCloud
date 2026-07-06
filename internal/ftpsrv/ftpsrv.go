// Package ftpsrv exposes the storage layer over FTP / FTPS.
package ftpsrv

import (
	"crypto/tls"
	"errors"
	"log/slog"

	ftpserver "github.com/fclairamb/ftpserverlib"

	"goblin_cloud/internal/auth"
	"goblin_cloud/internal/config"
	"goblin_cloud/internal/storage"
)

// driver implements ftpserver.MainDriver over the shared storage and auth.
type driver struct {
	store     *storage.Store
	auth      *auth.Authenticator
	settings  *ftpserver.Settings
	tlsConfig *tls.Config
}

// New builds an FTP server. tlsConfig may be nil when FTPS is disabled.
func New(store *storage.Store, a *auth.Authenticator, cfg config.Config, tlsConfig *tls.Config) (*ftpserver.FtpServer, error) {
	start, end, err := cfg.PassivePortRange()
	if err != nil {
		return nil, err
	}
	if cfg.FTP.TLS && tlsConfig == nil {
		return nil, errors.New("ftp.tls is enabled but no certificate is available")
	}

	tlsReq := ftpserver.ClearOrEncrypted
	if cfg.FTP.TLS {
		tlsReq = ftpserver.MandatoryEncryption
	}

	d := &driver{
		store:     store,
		auth:      a,
		tlsConfig: tlsConfig,
		settings: &ftpserver.Settings{
			ListenAddr:               cfg.FTP.Listen,
			Banner:                   "Goblin Cloud FTP",
			PassiveTransferPortRange: &ftpserver.PortRange{Start: start, End: end},
			TLSRequired:              tlsReq,
		},
	}

	srv := ftpserver.NewFtpServer(d)
	srv.Logger = slog.Default().With("svc", "ftp")
	return srv, nil
}

func (d *driver) GetSettings() (*ftpserver.Settings, error) { return d.settings, nil }

func (d *driver) ClientConnected(ftpserver.ClientContext) (string, error) {
	return "Goblin Cloud", nil
}

func (d *driver) ClientDisconnected(ftpserver.ClientContext) {}

// AuthUser accepts any username; the password must match the shared credential.
func (d *driver) AuthUser(_ ftpserver.ClientContext, _, pass string) (ftpserver.ClientDriver, error) {
	if !d.auth.CheckPassword(pass) {
		return nil, errors.New("invalid credentials")
	}
	return &clientFS{store: d.store}, nil
}

func (d *driver) GetTLSConfig() (*tls.Config, error) {
	if d.tlsConfig == nil {
		return nil, errors.New("TLS not configured")
	}
	return d.tlsConfig, nil
}
