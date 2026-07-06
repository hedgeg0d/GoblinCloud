// Package httpapi serves the REST API and the embedded web UI.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"goblin_cloud/internal/auth"
	"goblin_cloud/internal/storage"
	"goblin_cloud/internal/web"
)

const cookieName = "gcsession"

// Info is the server metadata exposed to the web UI (version, FTP details).
type Info struct {
	Version    string `json:"version"`
	FTPEnabled bool   `json:"ftpEnabled"`
	FTPPort    int    `json:"ftpPort"`
	FTPTLS     bool   `json:"ftpTLS"`
}

// Server bundles the dependencies the HTTP handlers need.
type Server struct {
	store  *storage.Store
	auth   *auth.Authenticator
	secure bool // set Secure flag on the session cookie (HTTPS)
	info   Info
}

// New builds the HTTP handler set. secure marks cookies for HTTPS deployments.
func New(store *storage.Store, a *auth.Authenticator, secure bool, info Info) http.Handler {
	s := &Server{store: store, auth: a, secure: secure, info: info}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.Handle("GET /api/info", s.guard(s.handleInfo))
	mux.Handle("GET /api/files", s.guard(s.handleList))
	mux.Handle("DELETE /api/files", s.guard(s.handleDelete))
	mux.Handle("POST /api/folder", s.guard(s.handleFolder))
	mux.Handle("POST /api/upload", s.guard(s.handleUpload))
	mux.Handle("GET /api/download", s.guard(s.handleDownload))
	mux.Handle("POST /api/rename", s.guard(s.handleRename))
	mux.Handle("/", web.Handler())
	return logMiddleware(mux)
}

// logMiddleware records one access-log line per request at debug level, so the
// configured log level controls request-level verbosity.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"dur", time.Since(start).String(),
			"remote", r.RemoteAddr,
		)
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code and the
// number of bytes written for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// ---- auth plumbing ----

func (s *Server) authed(r *http.Request) bool {
	if !s.auth.Enabled {
		return true
	}
	if c, err := r.Cookie(cookieName); err == nil && s.auth.ValidSession(c.Value) {
		return true
	}
	if _, pass, ok := r.BasicAuth(); ok && s.auth.CheckPassword(pass) {
		return true
	}
	return false
}

func (s *Server) guard(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		h(w, r)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !s.auth.CheckPassword(body.Password) {
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    s.auth.NewSession(),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.auth.EndSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// ---- file handlers ----

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.info)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	p := reqPath(r)
	entries, err := s.store.List(p)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "entries": entries})
}

func (s *Server) handleFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.Path == "" {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.store.Mkdir(body.Path); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Remove(reqPath(r)); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil || body.From == "" || body.To == "" {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.store.Rename(body.From, body.To); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	dir := reqPath(r)
	reader, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "expected multipart/form-data")
		return
	}
	stored := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "malformed upload")
			return
		}
		name := part.FileName()
		if name == "" {
			continue
		}
		dst := path.Join(dir, path.Base(name))
		f, err := s.store.Create(dst)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if _, err := io.Copy(f, part); err != nil {
			f.Close()
			writeErr(w, http.StatusInternalServerError, "write failed")
			return
		}
		if err := f.Close(); err != nil {
			writeErr(w, http.StatusInternalServerError, "write failed")
			return
		}
		stored++
	}
	writeJSON(w, http.StatusCreated, map[string]any{"stored": stored})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	p := reqPath(r)
	f, err := s.store.OpenRead(p)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeErr(w, http.StatusBadRequest, "not a file")
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(p)+"\"")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// ---- helpers ----

func reqPath(r *http.Request) string {
	p := r.URL.Query().Get("path")
	if p == "" {
		return "/"
	}
	return p
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNoSpace):
		writeErr(w, http.StatusInsufficientStorage, "no storage space left")
	case errors.Is(err, storage.ErrNotExist):
		writeErr(w, http.StatusNotFound, "not found")
	case strings.Contains(err.Error(), "exist"):
		writeErr(w, http.StatusConflict, "already exists")
	case strings.Contains(err.Error(), "invalid path"):
		writeErr(w, http.StatusBadRequest, "invalid path")
	default:
		slog.Default().Error("storage error", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}
