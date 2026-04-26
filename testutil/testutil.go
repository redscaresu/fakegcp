package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/redscaresu/fakegcp/handlers"
	"github.com/redscaresu/fakegcp/repository"
)

// NewTestServer creates an in-memory fakegcp server for testing.
func NewTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	f, err := os.CreateTemp("", "fakegcp-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	dbPath := f.Name()
	f.Close()

	repo, err := repository.New(dbPath)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("open db: %v", err)
	}

	app := handlers.NewApplication(repo)
	r := chi.NewRouter()
	app.RegisterRoutes(r)

	srv := httptest.NewServer(r)
	cleanup := func() {
		srv.Close()
		os.Remove(dbPath)
	}
	return srv, cleanup
}

func doJSON(t *testing.T, srv *httptest.Server, method, path string, body any) (*http.Response, map[string]any) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, srv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Add auth header for non-admin paths
	if len(path) < 5 || path[:5] != "/mock" {
		req.Header.Set("Authorization", "Bearer test-token")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}

	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if len(raw) == 0 {
		return resp, nil
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response (status %d, body %q): %v", resp.StatusCode, string(raw), err)
	}
	return resp, out
}

// DoCreate sends a POST request.
func DoCreate(t *testing.T, srv *httptest.Server, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, srv, http.MethodPost, path, body)
}

// DoGet sends a GET request.
func DoGet(t *testing.T, srv *httptest.Server, path string) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, srv, http.MethodGet, path, nil)
}

// DoDelete sends a DELETE request.
func DoDelete(t *testing.T, srv *httptest.Server, path string) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, srv, http.MethodDelete, path, nil)
}

// DoPatch sends a PATCH request.
func DoPatch(t *testing.T, srv *httptest.Server, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, srv, http.MethodPatch, path, body)
}

// DoPut sends a PUT request.
func DoPut(t *testing.T, srv *httptest.Server, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, srv, http.MethodPut, path, body)
}

// ResetState calls POST /mock/reset.
func ResetState(t *testing.T, srv *httptest.Server) {
	t.Helper()
	resp, _ := doJSON(t, srv, http.MethodPost, "/mock/reset", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("reset failed: %d", resp.StatusCode)
	}
}

// ComputePath builds a compute API path.
func ComputePath(project string, parts ...string) string {
	path := fmt.Sprintf("/compute/v1/projects/%s", project)
	for _, p := range parts {
		path += "/" + p
	}
	return path
}

// ContainerPath builds a container API path.
func ContainerPath(project, location string, parts ...string) string {
	path := fmt.Sprintf("/v1/projects/%s/locations/%s", project, location)
	for _, p := range parts {
		path += "/" + p
	}
	return path
}

// SQLPath builds a Cloud SQL API path.
func SQLPath(project string, parts ...string) string {
	path := fmt.Sprintf("/sql/v1beta4/projects/%s", project)
	for _, p := range parts {
		path += "/" + p
	}
	return path
}

// IAMPath builds an IAM API path.
func IAMPath(project string, parts ...string) string {
	path := fmt.Sprintf("/v1/projects/%s", project)
	for _, p := range parts {
		path += "/" + p
	}
	return path
}
