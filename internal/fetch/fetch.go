// Package fetch holds the HTTP client tai uses to pull remote content, along
// with the size, scheme, and redirect limits every such pull is subject to.
//
// It exists as a leaf package because two callers need identical behavior from
// opposite ends of the dependency graph: url-based skill includes in
// internal/install, and url sources in internal/source. install already imports
// source, so the client cannot live in either one.
package fetch

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	// DefaultTimeout bounds a single fetch, connection through body.
	DefaultTimeout = 30 * time.Second

	// MaxBytes caps a fetched file at 10MB. Remote content is unvalidated
	// input, and tai writes it to the user's disk.
	MaxBytes = 10 * 1024 * 1024

	// maxRedirects bounds a redirect chain, guarding against loops the
	// client's timeout would otherwise have to catch.
	maxRedirects = 10
)

// ParseURL parses raw and rejects any scheme tai will not fetch over.
// Callers should use it at config-parse time so a malformed URL is a config
// error rather than a mid-install failure.
func ParseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("URL %q has invalid scheme %q (must be http or https)", raw, parsed.Scheme)
	}
	return parsed, nil
}

// DefaultClient returns the HTTP client tai fetches remote content with.
func DefaultClient() *http.Client {
	return &http.Client{
		Timeout: DefaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects")
			}
			// The scheme check on the original URL says nothing about where a
			// redirect lands, so re-check every hop.
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-HTTP scheme %q not allowed", req.URL.Scheme)
			}
			return nil
		},
	}
}

// ToFile fetches rawURL into destPath, enforcing MaxBytes. destPath's parent
// directory must already exist. userAgent may be empty.
//
// An oversized response leaves no file behind: the cap is only detectable
// after MaxBytes have been written, so the partial file is removed before the
// error returns.
func ToFile(c *http.Client, rawURL, destPath, userAgent string) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("building request for %q: %w", rawURL, err)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %q: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %q: HTTP %d %s", rawURL, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating destination file %q: %w", destPath, err)
	}
	defer func() { _ = destFile.Close() }()

	// Read one byte past the cap so an exactly-MaxBytes file still succeeds
	// while anything larger is detectable.
	written, err := io.Copy(destFile, io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return fmt.Errorf("writing to %q: %w", destPath, err)
	}
	if written > MaxBytes {
		_ = destFile.Close()
		_ = os.Remove(destPath)
		return fmt.Errorf("URL %q exceeds maximum size of %d bytes", rawURL, MaxBytes)
	}

	return nil
}
