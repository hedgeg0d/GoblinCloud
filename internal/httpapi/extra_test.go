package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
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
