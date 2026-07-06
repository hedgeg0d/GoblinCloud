package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"goblin_cloud/internal/auth"
	"goblin_cloud/internal/httpapi"
	"goblin_cloud/internal/storage"
)

const testPassword = "s3cret"

func newServer(t *testing.T, minFree uint64) (*httptest.Server, *http.Client) {
	t.Helper()
	store, err := storage.New([]string{t.TempDir(), t.TempDir()}, minFree)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword(testPassword)
	a := auth.New(true, hash)
	info := httpapi.Info{Version: "test", FTPEnabled: true, FTPPort: 2121, FTPTLS: false}
	srv := httptest.NewServer(httpapi.New(store, a, false, info))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	return srv, &http.Client{Jar: jar}
}

func login(t *testing.T, srv *httptest.Server, c *http.Client, pw string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": pw})
	res, err := c.Post(srv.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func uploadFile(t *testing.T, srv *httptest.Server, c *http.Client, dir, name, content string) int {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	w, _ := mw.CreateFormFile("file", name)
	io.WriteString(w, content)
	mw.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/upload?path="+url.QueryEscape(dir), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func listNames(t *testing.T, srv *httptest.Server, c *http.Client, dir string) []string {
	t.Helper()
	res, err := c.Get(srv.URL + "/api/files?path=" + url.QueryEscape(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("list %s -> %d", dir, res.StatusCode)
	}
	var out struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	json.NewDecoder(res.Body).Decode(&out)
	names := make([]string, len(out.Entries))
	for i, e := range out.Entries {
		names[i] = e.Name
	}
	return names
}

func TestLoginAndGuard(t *testing.T) {
	srv, c := newServer(t, 0)

	// No auth -> 401.
	res, _ := c.Get(srv.URL + "/api/files?path=/")
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth list = %d, want 401", res.StatusCode)
	}

	if code := login(t, srv, c, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", code)
	}
	if code := login(t, srv, c, testPassword); code != http.StatusNoContent {
		t.Fatalf("good login = %d, want 204", code)
	}

	// Now authorised via cookie.
	res, _ = c.Get(srv.URL + "/api/files?path=/")
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authed list = %d, want 200", res.StatusCode)
	}
}

func TestBasicAuth(t *testing.T) {
	srv, _ := newServer(t, 0)
	c := &http.Client{} // no cookie jar
	req, _ := http.NewRequest("GET", srv.URL+"/api/files?path=/", nil)
	req.SetBasicAuth("anyuser", testPassword)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("basic-auth list = %d, want 200", res.StatusCode)
	}
}

func TestFileLifecycle(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)

	// mkdir
	body, _ := json.Marshal(map[string]string{"path": "/docs"})
	res, _ := c.Post(srv.URL+"/api/folder", "application/json", bytes.NewReader(body))
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("mkdir = %d, want 201", res.StatusCode)
	}

	// upload
	if code := uploadFile(t, srv, c, "/docs", "hello.txt", "hi there"); code != http.StatusCreated {
		t.Fatalf("upload = %d, want 201", code)
	}

	// list
	if names := listNames(t, srv, c, "/docs"); len(names) != 1 || names[0] != "hello.txt" {
		t.Fatalf("list = %v", names)
	}

	// download
	res, _ = c.Get(srv.URL + "/api/download?path=" + url.QueryEscape("/docs/hello.txt"))
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "hi there" {
		t.Fatalf("download body = %q", b)
	}

	// rename
	body, _ = json.Marshal(map[string]string{"from": "/docs/hello.txt", "to": "/docs/renamed.txt"})
	res, _ = c.Post(srv.URL+"/api/rename", "application/json", bytes.NewReader(body))
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("rename = %d, want 200", res.StatusCode)
	}
	if names := listNames(t, srv, c, "/docs"); len(names) != 1 || names[0] != "renamed.txt" {
		t.Fatalf("after rename = %v", names)
	}

	// delete
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/files?path="+url.QueryEscape("/docs/renamed.txt"), nil)
	res, _ = c.Do(req)
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", res.StatusCode)
	}
	if names := listNames(t, srv, c, "/docs"); len(names) != 0 {
		t.Fatalf("after delete = %v", names)
	}
}

func TestUploadNoSpaceReturns507(t *testing.T) {
	// Absurd min_free makes every root ineligible for writes.
	srv, c := newServer(t, 1<<62)
	login(t, srv, c, testPassword)
	if code := uploadFile(t, srv, c, "/", "big.bin", "x"); code != http.StatusInsufficientStorage {
		t.Fatalf("no-space upload = %d, want 507", code)
	}
}

func TestDownloadMissing404(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)
	res, _ := c.Get(srv.URL + "/api/download?path=" + url.QueryEscape("/nope.txt"))
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing download = %d, want 404", res.StatusCode)
	}
}

func TestUploadMultipleFiles(t *testing.T) {
	srv, c := newServer(t, 0)
	login(t, srv, c, testPassword)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		w, _ := mw.CreateFormFile("file", n)
		io.WriteString(w, "data-"+n)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/api/upload?path=/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, _ := c.Do(req)
	var out struct {
		Stored int `json:"stored"`
	}
	json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if out.Stored != 3 {
		t.Fatalf("stored = %d, want 3", out.Stored)
	}
	names := strings.Join(listNames(t, srv, c, "/"), ",")
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if !strings.Contains(names, n) {
			t.Fatalf("missing %s in %s", n, names)
		}
	}
}
