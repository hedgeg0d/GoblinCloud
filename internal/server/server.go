// Package server wires config into the storage, auth, HTTP and FTP layers and
// runs them together.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"goblin_cloud/internal/auth"
	"goblin_cloud/internal/config"
	"goblin_cloud/internal/ftpsrv"
	"goblin_cloud/internal/httpapi"
	"goblin_cloud/internal/storage"
)

// Run builds every front door from cfg and serves until a signal arrives.
func Run(cfg config.Config) error {
	minFree, err := cfg.MinFreeBytes()
	if err != nil {
		return err
	}
	store, err := storage.New(cfg.Storage.Paths, minFree)
	if err != nil {
		return err
	}
	a := auth.New(cfg.Auth.Enabled, cfg.Auth.PasswordHash)
	handler := httpapi.New(store, a, cfg.Global())

	var tlsConfig *tls.Config
	var certManager *autocert.Manager
	if cfg.Global() {
		certManager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.Server.Domain),
			Cache:      autocert.DirCache(cfg.Server.AutocertCache),
			Email:      cfg.Server.AutocertEmail,
		}
		tlsConfig = certManager.TLSConfig()
	}

	// FTP (optional) runs in the background.
	var ftp interface{ Stop() error }
	if cfg.FTP.Enabled {
		srv, err := ftpsrv.New(store, a, cfg, tlsConfig)
		if err != nil {
			return err
		}
		ftp = srv
		go func() {
			slog.Info("ftp listening", "addr", cfg.FTP.Listen, "tls", cfg.FTP.TLS)
			if err := srv.ListenAndServe(); err != nil {
				slog.Error("ftp server stopped", "err", err)
			}
		}()
	}

	// HTTP / HTTPS.
	httpErr := make(chan error, 2)
	var httpsSrv *http.Server

	if cfg.Global() {
		// :80 handles the ACME challenge and redirects to HTTPS.
		challenge := &http.Server{
			Addr:              ":80",
			Handler:           certManager.HTTPHandler(nil),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() { httpErr <- challenge.ListenAndServe() }()

		httpsSrv = &http.Server{
			Addr:              ":443",
			Handler:           handler,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 30 * time.Second,
		}
		go func() {
			slog.Info("https listening", "domain", cfg.Server.Domain)
			httpErr <- httpsSrv.ListenAndServeTLS("", "")
		}()
	} else {
		httpsSrv = &http.Server{
			Addr:              cfg.Server.Listen,
			Handler:           handler,
			ReadHeaderTimeout: 30 * time.Second,
		}
		go func() {
			slog.Info("http listening", "addr", cfg.Server.Listen)
			httpErr <- httpsSrv.ListenAndServe()
		}()
	}

	// Wait for a signal or a fatal server error.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
	case err := <-httpErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if httpsSrv != nil {
		_ = httpsSrv.Shutdown(ctx)
	}
	if ftp != nil {
		_ = ftp.Stop()
	}
	return nil
}
