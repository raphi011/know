package webdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler_OSMetadataFastPath(t *testing.T) {
	// Handler with nil dependencies — if the fast path works,
	// these are never touched and we don't panic.
	handler := NewHandler(context.Background(), "/dav/", nil, nil, nil, nil, true, 0)

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
		// LOCK on ._ files should return 200 with fake lock token
		{"LOCK", "/dav/somevault/._file.md", http.StatusOK},
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

	// Verify LOCK returns Lock-Token header
	t.Run("LOCK Lock-Token header", func(t *testing.T) {
		req := httptest.NewRequest("LOCK", "/dav/somevault/.DS_Store", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
		if rec.Header().Get("Lock-Token") == "" {
			t.Error("expected Lock-Token header in LOCK response")
		}
	})
}

func TestHandler_UnsupportedFileFastPath(t *testing.T) {
	// Handler with nil dependencies — if the fast path works,
	// these are never touched and we don't panic.
	handler := NewHandler(context.Background(), "/dav/", nil, nil, nil, nil, true, 0)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		// PUT on unsupported files should return 201 (accepted, discarded)
		{"PUT", "/dav/somevault/doc.pdf", http.StatusCreated},
		{"PUT", "/dav/somevault/notes.txt", http.StatusCreated},
		{"PUT", "/dav/somevault/report.docx", http.StatusCreated},
		// PROPFIND/GET/HEAD on unsupported files should return 404
		{"PROPFIND", "/dav/somevault/doc.pdf", http.StatusNotFound},
		{"GET", "/dav/somevault/doc.pdf", http.StatusNotFound},
		{"HEAD", "/dav/somevault/doc.pdf", http.StatusNotFound},
		// LOCK on unsupported files should return 200 with fake lock token
		{"LOCK", "/dav/somevault/doc.pdf", http.StatusOK},
		// UNLOCK/DELETE on unsupported files should return 204
		{"UNLOCK", "/dav/somevault/doc.pdf", http.StatusNoContent},
		{"DELETE", "/dav/somevault/doc.pdf", http.StatusNoContent},
		// COPY/MOVE on unsupported files should return 204 (nothing to copy/move)
		{"COPY", "/dav/somevault/doc.pdf", http.StatusNoContent},
		{"MOVE", "/dav/somevault/doc.pdf", http.StatusNoContent},
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

	// Verify LOCK returns Lock-Token header
	t.Run("LOCK Lock-Token header", func(t *testing.T) {
		req := httptest.NewRequest("LOCK", "/dav/somevault/doc.pdf", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
		if rec.Header().Get("Lock-Token") == "" {
			t.Error("expected Lock-Token header in LOCK response")
		}
	})
}

func TestHandler_ImageFilesNotShortCircuited(t *testing.T) {
	// Image files (.png, .jpg) are supported — they should NOT be caught
	// by the unsupported file fast path. With nil deps, reaching the real
	// handler will panic, proving the fast path didn't intercept.
	handler := NewHandler(context.Background(), "/dav/", nil, nil, nil, nil, true, 0)
	req := httptest.NewRequest("PROPFIND", "/dav/somevault/photo.png", nil)
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
		t.Error("expected panic for .png file (proves request reached auth/DB code with nil deps)")
	}
}

func TestHandler_FolderPathsNotShortCircuited(t *testing.T) {
	// Extensionless paths (likely directories) should NOT be caught by the
	// unsupported file fast path. With nil deps, reaching the real handler
	// will panic, proving the fast path didn't intercept.
	handler := NewHandler(context.Background(), "/dav/", nil, nil, nil, nil, true, 0)
	req := httptest.NewRequest("PROPFIND", "/dav/somevault/notes", nil)
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
		t.Error("expected panic for extensionless path (proves request reached auth/DB code with nil deps)")
	}
}

func TestHandler_RealFilesNotShortCircuited(t *testing.T) {
	// A request for a real .md file with nil deps should panic,
	// proving the fast path did NOT intercept it.
	handler := NewHandler(context.Background(), "/dav/", nil, nil, nil, nil, true, 0)
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
