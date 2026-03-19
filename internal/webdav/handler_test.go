package webdav

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSingleWriteResponseWriter_WriteHeader(t *testing.T) {
	t.Run("first WriteHeader passes through", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := &singleWriteResponseWriter{ResponseWriter: rec}
		w.WriteHeader(http.StatusNotFound)
		if rec.Code != http.StatusNotFound {
			t.Errorf("got %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("second WriteHeader is suppressed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := &singleWriteResponseWriter{ResponseWriter: rec}
		w.WriteHeader(http.StatusOK)
		w.WriteHeader(http.StatusInternalServerError) // should be suppressed
		if rec.Code != http.StatusOK {
			t.Errorf("got %d, want %d (second call should be suppressed)", rec.Code, http.StatusOK)
		}
	})

	t.Run("Write without WriteHeader sends 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := &singleWriteResponseWriter{ResponseWriter: rec}
		_, err := w.Write([]byte("hello"))
		if err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("got %d, want %d", rec.Code, http.StatusOK)
		}
		if !w.wroteHeader {
			t.Error("expected wroteHeader to be true after Write")
		}
		if w.statusCode != http.StatusOK {
			t.Errorf("statusCode got %d, want %d", w.statusCode, http.StatusOK)
		}
	})

	t.Run("WriteHeader then Write does not override status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := &singleWriteResponseWriter{ResponseWriter: rec}
		w.WriteHeader(http.StatusCreated)
		_, err := w.Write([]byte("body"))
		if err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusCreated {
			t.Errorf("got %d, want %d", rec.Code, http.StatusCreated)
		}
	})

	t.Run("Unwrap returns underlying writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := &singleWriteResponseWriter{ResponseWriter: rec}
		if w.Unwrap() != rec {
			t.Error("Unwrap did not return the underlying ResponseWriter")
		}
	})
}

func TestHandler_NonStoredFileFastPath(t *testing.T) {
	// Handler with nil dependencies — if the fast path works,
	// these are never touched and we don't panic.
	handler := NewHandler(context.Background(), "/dav/", nil, nil, nil, nil, true, 0, nil)

	tests := []struct {
		method string
		path   string
		want   int
	}{
		// OS metadata files
		{"PROPFIND", "/dav/somevault/._file.md", http.StatusNotFound},
		{"PROPFIND", "/dav/somevault/._.", http.StatusNotFound},
		{"PROPFIND", "/dav/somevault/.DS_Store", http.StatusNotFound},
		{"GET", "/dav/somevault/._file.md", http.StatusNotFound},
		{"HEAD", "/dav/somevault/._file.md", http.StatusNotFound},
		{"PUT", "/dav/somevault/._file.md", http.StatusCreated},
		{"PUT", "/dav/somevault/.DS_Store", http.StatusCreated},
		{"LOCK", "/dav/somevault/._file.md", http.StatusOK},
		{"UNLOCK", "/dav/somevault/._file.md", http.StatusNoContent},
		{"DELETE", "/dav/somevault/._file.md", http.StatusNoContent},
		// Unsupported file types
		{"PUT", "/dav/somevault/doc.pdf", http.StatusCreated},
		{"PUT", "/dav/somevault/notes.txt", http.StatusCreated},
		{"PUT", "/dav/somevault/report.docx", http.StatusCreated},
		{"PROPFIND", "/dav/somevault/doc.pdf", http.StatusNotFound},
		{"GET", "/dav/somevault/doc.pdf", http.StatusNotFound},
		{"HEAD", "/dav/somevault/doc.pdf", http.StatusNotFound},
		{"LOCK", "/dav/somevault/doc.pdf", http.StatusOK},
		{"UNLOCK", "/dav/somevault/doc.pdf", http.StatusNoContent},
		{"DELETE", "/dav/somevault/doc.pdf", http.StatusNoContent},
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
	for _, lockPath := range []string{"/dav/somevault/.DS_Store", "/dav/somevault/doc.pdf"} {
		t.Run("LOCK Lock-Token "+lockPath, func(t *testing.T) {
			req := httptest.NewRequest("LOCK", lockPath, nil)
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
}

// TestHandler_SupportedFilesNotShortCircuited verifies that supported file types
// (.md, .png) and extensionless paths (directories) are NOT caught by the
// non-stored file fast path. With nil deps, reaching the real handler panics,
// proving the fast path didn't intercept.
func TestHandler_SupportedFilesNotShortCircuited(t *testing.T) {
	handler := NewHandler(context.Background(), "/dav/", nil, nil, nil, nil, true, 0, nil)

	tests := []struct {
		path string
		desc string
	}{
		{"/dav/somevault/photo.png", "image file"},
		{"/dav/somevault/notes", "extensionless path (directory)"},
		{"/dav/somevault/readme.md", "markdown file"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			req := httptest.NewRequest("PROPFIND", tt.path, nil)
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
				t.Errorf("expected panic for %s (proves request reached auth/DB code with nil deps)", tt.desc)
			}
		})
	}
}
