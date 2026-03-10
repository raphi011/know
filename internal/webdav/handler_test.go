package webdav

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_OSMetadataFastPath(t *testing.T) {
	// Handler with nil dependencies — if the fast path works,
	// these are never touched and we don't panic.
	handler := NewHandler("/dav/", nil, nil, nil, true, 0)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		// PROPFIND/GET/HEAD on ._ files should return 404 without hitting auth
		{"PROPFIND", "/dav/somevault/._file.md", http.StatusNotFound},
		{"PROPFIND", "/dav/somevault/._.", http.StatusNotFound},
		{"PROPFIND", "/dav/somevault/.DS_Store", http.StatusNotFound},
		{"GET", "/dav/somevault/._file.md", http.StatusNotFound},
		{"HEAD", "/dav/somevault/._file.md", http.StatusNotFound},
		// PUT on ._ files should return 201 (accepted, discarded)
		{"PUT", "/dav/somevault/._file.md", http.StatusCreated},
		{"PUT", "/dav/somevault/.DS_Store", http.StatusCreated},
		// LOCK on ._ files should return 423 (rejected)
		{"LOCK", "/dav/somevault/._file.md", http.StatusLocked},
		// UNLOCK on ._ files should return 204
		{"UNLOCK", "/dav/somevault/._file.md", http.StatusNoContent},
		// DELETE on ._ files should return 204
		{"DELETE", "/dav/somevault/._file.md", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestHandler_RealFilesNotShortCircuited(t *testing.T) {
	// A request for a real .md file with nil deps should panic,
	// proving the fast path did NOT intercept it.
	handler := NewHandler("/dav/", nil, nil, nil, true, 0)
	req := httptest.NewRequest("PROPFIND", "/dav/somevault/readme.md", nil)
	rec := httptest.NewRecorder()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		handler.ServeHTTP(rec, req)
	}()

	if !panicked {
		t.Error("expected panic for real .md file (proves request reached auth/DB code with nil deps)")
	}
}
