// Package httpapi serves the REST API and the embedded web UI.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"goblin_cloud/internal/auth"
	"goblin_cloud/internal/storage"
	"goblin_cloud/internal/web"
)

const cookieName = "gcsession"

// notesDir is the hidden logical folder where web-UI notes live. It sits inside
// the normal storage tree but is filtered out of file listings so notes never
// show up as files.
const notesDir = "/.web_interface_notes"

// Info is the server metadata exposed to the web UI (version, FTP details).
type Info struct {
	Version     string `json:"version"`
	AuthEnabled bool   `json:"authEnabled"`
	FTPEnabled  bool   `json:"ftpEnabled"`
	FTPPort     int    `json:"ftpPort"`
	FTPTLS      bool   `json:"ftpTLS"`
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
	mux.Handle("GET /api/notes", s.guard(s.handleNotesList))
	mux.Handle("POST /api/notes", s.guard(s.handleNoteSave))
	mux.Handle("DELETE /api/notes", s.guard(s.handleNoteDelete))
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
	// Hide the notes folder so notes never appear among files.
	kept := entries[:0]
	for _, e := range entries {
		if "/"+e.Name == notesDir {
			continue
		}
		kept = append(kept, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "entries": kept})
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

// note is a single web-UI note: a titled scratch of text stored as a JSON file
// under notesDir. The list view shows only Title; Body holds the text.
type note struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Updated time.Time `json:"updated"`
}

// validNoteID reports whether id is safe to use as a filename (no path tricks).
func validNoteID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func notePath(id string) string { return notesDir + "/" + id + ".json" }

// handleNotesList returns every note (title and body), newest first. A missing
// notes folder simply means no notes yet.
func (s *Server) handleNotesList(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.List(notesDir)
	if err != nil {
		if errors.Is(err, storage.ErrNotExist) {
			writeJSON(w, http.StatusOK, map[string]any{"notes": []note{}})
			return
		}
		writeStoreErr(w, err)
		return
	}
	notes := make([]note, 0, len(entries))
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		f, err := s.store.OpenRead(notesDir + "/" + e.Name)
		if err != nil {
			continue
		}
		var n note
		dec := json.NewDecoder(io.LimitReader(f, 8<<20))
		derr := dec.Decode(&n)
		f.Close()
		if derr == nil {
			notes = append(notes, n)
		}
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Updated.After(notes[j].Updated) })
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

// handleNoteSave creates a note (no id) or overwrites an existing one (id set).
func (s *Server) handleNoteSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	id := body.ID
	if id == "" {
		id = strconv.FormatInt(time.Now().UnixNano(), 36)
	} else if !validNoteID(id) {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	n := note{ID: id, Title: body.Title, Body: body.Body, Updated: time.Now()}
	data, _ := json.Marshal(n)
	f, err := s.store.Create(notePath(id))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		writeErr(w, http.StatusInternalServerError, "write failed")
		return
	}
	if err := f.Close(); err != nil {
		writeErr(w, http.StatusInternalServerError, "write failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// handleNoteDelete removes one note by id.
func (s *Server) handleNoteDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validNoteID(id) {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.Remove(notePath(id)); err != nil {
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
