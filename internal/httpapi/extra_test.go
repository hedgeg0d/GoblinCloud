package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLogout(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)

	// Authorised before logout.
	res, _ := c.Get(srv.URL + "/api/files?path=/")
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pre-logout list = %d", res.StatusCode)
	}

	res, _ = c.Post(srv.URL+"/api/logout", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", res.StatusCode)
	}

	// Cookie invalidated -> 401.
	res, _ = c.Get(srv.URL + "/api/files?path=/")
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-logout list = %d, want 401", res.StatusCode)
	}
}

func TestBadRequests(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)

	post := func(path, body string) int {
		res, _ := c.Post(srv.URL+path, "application/json", strings.NewReader(body))
		res.Body.Close()
		return res.StatusCode
	}

	if code := post("/api/folder", "{ not json"); code != http.StatusBadRequest {
		t.Errorf("folder bad json = %d, want 400", code)
	}
	if code := post("/api/folder", `{"path":""}`); code != http.StatusBadRequest {
		t.Errorf("folder empty path = %d, want 400", code)
	}
	if code := post("/api/rename", "{ not json"); code != http.StatusBadRequest {
		t.Errorf("rename bad json = %d, want 400", code)
	}
	if code := post("/api/rename", `{"from":"/a"}`); code != http.StatusBadRequest {
		t.Errorf("rename missing to = %d, want 400", code)
	}
	if code := post("/api/login", "{ not json"); code != http.StatusBadRequest {
		t.Errorf("login bad json = %d, want 400", code)
	}

	// Upload without multipart body.
	res, _ := c.Post(srv.URL+"/api/upload?path=/", "text/plain", strings.NewReader("hi"))
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("non-multipart upload = %d, want 400", res.StatusCode)
	}
}

func TestFolderConflict(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	body, _ := json.Marshal(map[string]string{"path": "/dup"})

	res, _ := c.Post(srv.URL+"/api/folder", "application/json", bytes.NewReader(body))
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("first mkdir = %d", res.StatusCode)
	}
	res, _ = c.Post(srv.URL+"/api/folder", "application/json", bytes.NewReader(body))
	res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate mkdir = %d, want 409", res.StatusCode)
	}
}

func TestRenameMissing404(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	body, _ := json.Marshal(map[string]string{"from": "/ghost.txt", "to": "/x.txt"})
	res, _ := c.Post(srv.URL+"/api/rename", "application/json", bytes.NewReader(body))
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("rename missing = %d, want 404", res.StatusCode)
	}
}

func TestDeleteMissing404(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/files?path="+url.QueryEscape("/ghost"), nil)
	res, _ := c.Do(req)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing = %d, want 404", res.StatusCode)
	}
}

func TestInvalidPathRejected(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	// A NUL byte in the path must be rejected by the storage layer -> 400.
	res, _ := c.Get(srv.URL + "/api/download?path=%2Ffoo%00bar")
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid path = %d, want 400", res.StatusCode)
	}
}

func TestAccessLogEmittedAtDebug(t *testing.T) {
	// Capture the default logger at debug so the access-log middleware fires.
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	srv, c := newServer(t, 0)
	res, _ := c.Get(srv.URL + "/api/files?path=/") // unauthenticated -> 401
	res.Body.Close()

	out := buf.String()
	if !strings.Contains(out, "msg=request") {
		t.Fatalf("expected an access-log line, got:\n%s", out)
	}
	if !strings.Contains(out, "method=GET") || !strings.Contains(out, "status=401") {
		t.Fatalf("access log missing method/status:\n%s", out)
	}
}

func TestNoAccessLogAtInfo(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	srv, c := newServer(t, 0)
	res, _ := c.Get(srv.URL + "/api/files?path=/")
	res.Body.Close()

	if strings.Contains(buf.String(), "msg=request") {
		t.Fatalf("access log should be suppressed at info level:\n%s", buf.String())
	}
}

func TestInfoEndpoint(t *testing.T) {
	srv, c := newServer(t, 0)

	// Guarded: no auth -> 401.
	res, _ := c.Get(srv.URL + "/api/info")
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth info = %d, want 401", res.StatusCode)
	}

	login(t, srv, c, testPassword)
	res, err := c.Get(srv.URL + "/api/info")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("info = %d, want 200", res.StatusCode)
	}
	var info struct {
		Version     string `json:"version"`
		AuthEnabled bool   `json:"authEnabled"`
		FTPEnabled  bool   `json:"ftpEnabled"`
		FTPPort     int    `json:"ftpPort"`
		FTPTLS      bool   `json:"ftpTLS"`
	}
	json.NewDecoder(res.Body).Decode(&info)
	if info.Version != "test" || !info.AuthEnabled || !info.FTPEnabled || info.FTPPort != 2121 {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestWrongBasicAuth(t *testing.T) {
	srv, _ := newServer(t, 0)
	c := &http.Client{}
	req, _ := http.NewRequest("GET", srv.URL+"/api/files?path=/", nil)
	req.SetBasicAuth("user", "wrongpass")
	res, _ := c.Do(req)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong basic-auth = %d, want 401", res.StatusCode)
	}
}

type apiNote struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Updated string `json:"updated"`
}

// saveNote posts a note and returns the status plus the id the server assigned.
func saveNote(t *testing.T, srv *httptest.Server, c *http.Client, id, title, body string) (int, string) {
	t.Helper()
	payload := map[string]string{"title": title, "body": body}
	if id != "" {
		payload["id"] = id
	}
	b, _ := json.Marshal(payload)
	res, err := c.Post(srv.URL+"/api/notes", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		ID string `json:"id"`
	}
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out.ID
}

func listNotes(t *testing.T, srv *httptest.Server, c *http.Client) []apiNote {
	t.Helper()
	res, err := c.Get(srv.URL + "/api/notes")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("list notes -> %d", res.StatusCode)
	}
	var out struct {
		Notes []apiNote `json:"notes"`
	}
	json.NewDecoder(res.Body).Decode(&out)
	return out.Notes
}

func TestNotesLifecycle(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)

	// Empty to start.
	if got := listNotes(t, srv, c); len(got) != 0 {
		t.Fatalf("fresh notes = %d, want 0", len(got))
	}

	code, id1 := saveNote(t, srv, c, "", "First", "body one")
	if code != http.StatusCreated || id1 == "" {
		t.Fatalf("create note = %d id=%q, want 201 + id", code, id1)
	}
	if code, _ := saveNote(t, srv, c, "", "Second", "body two"); code != http.StatusCreated {
		t.Fatalf("create second = %d, want 201", code)
	}

	notes := listNotes(t, srv, c)
	if len(notes) != 2 {
		t.Fatalf("notes = %d, want 2", len(notes))
	}
	// Newest first.
	if notes[0].Title != "Second" {
		t.Fatalf("first listed = %q, want Second", notes[0].Title)
	}

	// Editing by id overwrites in place, no new note.
	if code, id := saveNote(t, srv, c, id1, "First edited", "body one v2"); code != http.StatusCreated || id != id1 {
		t.Fatalf("edit note = %d id=%q, want 201 id=%q", code, id, id1)
	}
	notes = listNotes(t, srv, c)
	if len(notes) != 2 {
		t.Fatalf("after edit notes = %d, want 2", len(notes))
	}
	var edited *apiNote
	for i := range notes {
		if notes[i].ID == id1 {
			edited = &notes[i]
		}
	}
	if edited == nil || edited.Title != "First edited" || edited.Body != "body one v2" {
		t.Fatalf("edited note = %+v, want title/body updated", edited)
	}

	// Delete one.
	res, _ := c.Do(mustReq(t, "DELETE", srv.URL+"/api/notes?id="+url.QueryEscape(id1), nil))
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete note = %d, want 204", res.StatusCode)
	}
	if got := listNotes(t, srv, c); len(got) != 1 || got[0].Title != "Second" {
		t.Fatalf("after delete = %+v, want only Second", got)
	}
}

func TestNotesHiddenFromFiles(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	saveNote(t, srv, c, "", "hi", "text")
	for _, name := range listNames(t, srv, c, "/") {
		if strings.Contains(name, "web_interface_notes") {
			t.Fatalf("notes folder leaked into file listing: %q", name)
		}
	}
}

func TestNoteSaveRejectsEmptyTitle(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	if code, _ := saveNote(t, srv, c, "", "   ", "body"); code != http.StatusBadRequest {
		t.Fatalf("empty-title note = %d, want 400", code)
	}
}

func TestNoteDeleteInvalidID(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	res, _ := c.Do(mustReq(t, "DELETE", srv.URL+"/api/notes?id=../evil", nil))
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id delete = %d, want 400", res.StatusCode)
	}
}

func mustReq(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
