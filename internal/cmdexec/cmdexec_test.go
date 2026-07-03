package cmdexec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("cmdexec tests require POSIX sh")
	}
}

func spec(argv ...string) Spec {
	return Spec{Argv: argv, Timeout: 10 * time.Second}
}

func TestRun(t *testing.T) {
	skipOnWindows(t)

	t.Run("success", func(t *testing.T) {
		if err := Run(spec("sh", "-c", "true")); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	t.Run("nonzero exit includes code and stderr tail", func(t *testing.T) {
		err := Run(spec("sh", "-c", `printf 'boom on stderr' >&2; exit 7`))
		if err == nil {
			t.Fatal("expected error for nonzero exit")
		}
		if !strings.Contains(err.Error(), "exited 7") {
			t.Errorf("error should mention 'exited 7', got: %v", err)
		}
		if !strings.Contains(err.Error(), "boom on stderr") {
			t.Errorf("error should include stderr tail, got: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		err := Run(Spec{Argv: []string{"sh", "-c", "sleep 5"}, Timeout: 100 * time.Millisecond})
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("error should mention timeout, got: %v", err)
		}
	})

	t.Run("env vars set and inherited TAI_ stripped", func(t *testing.T) {
		t.Setenv("TAI_FOO", "stale")
		out := filepath.Join(t.TempDir(), "probe")
		s := Spec{
			Argv:    []string{"sh", "-c", `printf '%s|%s' "$TAI_TARGET" "${TAI_FOO:-unset}" > "$OUT"`},
			Timeout: 10 * time.Second,
			Env:     map[string]string{"TAI_TARGET": "claude"},
		}
		// OUT is inherited from the test process env.
		t.Setenv("OUT", out)
		if err := Run(s); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		got, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "claude|unset" {
			t.Errorf("probe = %q, want %q", string(got), "claude|unset")
		}
	})
}

func TestFetchInto(t *testing.T) {
	skipOnWindows(t)

	t.Run("stages via TAI_DEST_DIR and swaps into dest", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		s := spec("sh", "-c", `printf 'hello' > "$TAI_DEST_DIR/f.md"`)
		if err := FetchInto(s, dest, ""); err != nil {
			t.Fatalf("FetchInto() error = %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "f.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "hello" {
			t.Errorf("content = %q, want %q", string(got), "hello")
		}
	})

	t.Run("empty output rejected", func(t *testing.T) {
		err := FetchInto(spec("sh", "-c", "true"), filepath.Join(t.TempDir(), "out"), "")
		if err == nil {
			t.Fatal("expected error for empty output")
		}
		if !strings.Contains(err.Error(), "no files") {
			t.Errorf("error should mention 'no files', got: %v", err)
		}
	})

	t.Run("failure preserves previous dest", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		if err := os.MkdirAll(dest, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "keep.md"), []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}

		err := FetchInto(spec("sh", "-c", "exit 1"), dest, "")
		if err == nil {
			t.Fatal("expected error from failing script")
		}
		if !strings.Contains(err.Error(), "staging:") {
			t.Errorf("error should include staging path, got: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "keep.md"))
		if err != nil || string(got) != "original" {
			t.Errorf("previous dest content should survive failure, got %q, err %v", string(got), err)
		}
	})

	t.Run("stagingParent is honored", func(t *testing.T) {
		root := t.TempDir()
		stagingParent := filepath.Join(root, "staging")
		dest := filepath.Join(root, "out")

		s := spec("sh", "-c", `printf '%s' "$TAI_DEST_DIR" > "$TAI_DEST_DIR/where"`)
		if err := FetchInto(s, dest, stagingParent); err != nil {
			t.Fatalf("FetchInto() error = %v", err)
		}
		where, err := os.ReadFile(filepath.Join(dest, "where"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(where), stagingParent+string(filepath.Separator)) {
			t.Errorf("staging dir %q not under stagingParent %q", string(where), stagingParent)
		}
	})
}

func TestSwapInto(t *testing.T) {
	t.Run("replaces existing dest and removes backup", func(t *testing.T) {
		root := t.TempDir()
		staging := filepath.Join(root, "staging")
		dest := filepath.Join(root, "dest")
		mustWriteTree(t, staging, map[string]string{"fresh.md": "fresh"})
		mustWriteTree(t, dest, map[string]string{"stale.md": "stale"})

		if err := SwapInto(staging, dest); err != nil {
			t.Fatalf("SwapInto() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "stale.md")); !os.IsNotExist(err) {
			t.Errorf("stale.md should be gone, stat err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "fresh.md")); err != nil {
			t.Errorf("fresh.md should exist, err = %v", err)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.Contains(e.Name(), ".old-") {
				t.Errorf("backup dir %q should have been removed", e.Name())
			}
		}
	})

	t.Run("restores previous dest when incoming rename fails", func(t *testing.T) {
		root := t.TempDir()
		dest := filepath.Join(root, "dest")
		mustWriteTree(t, dest, map[string]string{"keep.md": "original"})

		// A nonexistent staging path makes the incoming rename fail after
		// dest has already been moved aside.
		err := SwapInto(filepath.Join(root, "does-not-exist"), dest)
		if err == nil {
			t.Fatal("expected error for missing staging")
		}
		got, readErr := os.ReadFile(filepath.Join(dest, "keep.md"))
		if readErr != nil || string(got) != "original" {
			t.Errorf("dest should be restored after failed swap, got %q, err %v", string(got), readErr)
		}
	})
}

func mustWriteTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSwapIntoSweepsStaleBackups(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	dest := filepath.Join(root, "dest")
	mustWriteTree(t, staging, map[string]string{"fresh.md": "fresh"})
	mustWriteTree(t, dest, map[string]string{"old.md": "old"})
	// Simulate a crashed prior run's orphaned backup (different pid).
	mustWriteTree(t, dest+".old-99999999", map[string]string{"ancient.md": "ancient"})

	if err := SwapInto(staging, dest); err != nil {
		t.Fatalf("SwapInto() error = %v", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".old-") {
			t.Errorf("stale backup %q should have been swept", e.Name())
		}
	}
}
