package fetch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "https", raw: "https://example.com/a.md"},
		{name: "http", raw: "http://example.com/a.md"},
		{name: "file scheme rejected", raw: "file:///etc/passwd", wantErr: "invalid scheme"},
		{name: "ftp rejected", raw: "ftp://example.com/a.md", wantErr: "invalid scheme"},
		{name: "no scheme rejected", raw: "example.com/a.md", wantErr: "invalid scheme"},
		{name: "unparseable rejected", raw: "https://exa mple.com/\x7f", wantErr: "invalid URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseURL(tt.raw)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseURL(%q) = %v, want nil", tt.raw, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseURL(%q) = nil, want error containing %q", tt.raw, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultClient(t *testing.T) {
	client := DefaultClient()

	if client.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", client.Timeout, DefaultTimeout)
	}
	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect should be configured")
	}
}

func TestToFile(t *testing.T) {
	t.Run("writes body to file", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("# Guide\n"))
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "guide.md")
		if err := ToFile(server.Client(), server.URL+"/guide.md", dest, ""); err != nil {
			t.Fatalf("ToFile() error = %v", err)
		}

		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("reading dest: %v", err)
		}
		if string(got) != "# Guide\n" {
			t.Errorf("content = %q, want %q", got, "# Guide\n")
		}
	})

	t.Run("sends user agent when set", func(t *testing.T) {
		var got string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("User-Agent")
			_, _ = w.Write([]byte("x"))
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "x.md")
		if err := ToFile(server.Client(), server.URL, dest, "tai/1.2.3"); err != nil {
			t.Fatalf("ToFile() error = %v", err)
		}
		if got != "tai/1.2.3" {
			t.Errorf("User-Agent = %q, want %q", got, "tai/1.2.3")
		}
	})

	t.Run("non-200 rejected", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "missing.md")
		err := ToFile(server.Client(), server.URL+"/missing.md", dest, "")
		if err == nil {
			t.Fatal("ToFile() = nil, want error for HTTP 404")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error = %v, want it to mention the status code", err)
		}
	})

	t.Run("exactly MaxBytes accepted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(make([]byte, MaxBytes))
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "exact.bin")
		if err := ToFile(server.Client(), server.URL, dest, ""); err != nil {
			t.Fatalf("ToFile() error = %v, want a file of exactly MaxBytes to be accepted", err)
		}
	})

	t.Run("oversized rejected and partial file removed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(make([]byte, MaxBytes+1))
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "large.bin")
		err := ToFile(server.Client(), server.URL, dest, "")
		if err == nil {
			t.Fatal("ToFile() = nil, want error for oversized response")
		}
		if !strings.Contains(err.Error(), "exceeds maximum size") {
			t.Errorf("error = %v, want it to mention the size limit", err)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Error("the partial file should have been removed")
		}
	})

	t.Run("redirect to non-http scheme blocked", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		}))
		defer server.Close()

		client := server.Client()
		client.CheckRedirect = DefaultClient().CheckRedirect

		dest := filepath.Join(t.TempDir(), "x.md")
		err := ToFile(client, server.URL, dest, "")
		if err == nil {
			t.Fatal("ToFile() = nil, want the redirect to a non-HTTP scheme to be blocked")
		}
		if !strings.Contains(err.Error(), "non-HTTP scheme") {
			t.Errorf("error = %v, want it to name the rejected scheme", err)
		}
	})

	t.Run("redirect loop blocked", func(t *testing.T) {
		var server *httptest.Server
		hops := 0
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops++
			http.Redirect(w, r, fmt.Sprintf("%s/hop%d", server.URL, hops), http.StatusFound)
		}))
		defer server.Close()

		client := server.Client()
		client.CheckRedirect = DefaultClient().CheckRedirect

		dest := filepath.Join(t.TempDir(), "x.md")
		err := ToFile(client, server.URL, dest, "")
		if err == nil {
			t.Fatal("ToFile() = nil, want the redirect loop to be blocked")
		}
		if !strings.Contains(err.Error(), "too many redirects") {
			t.Errorf("error = %v, want it to report too many redirects", err)
		}
	})
}
