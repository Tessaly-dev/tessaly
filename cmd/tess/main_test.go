// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tessaly-dev/tessaly/internal/store"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"version", []string{"version"}, 0, "tess dev\n", ""},
		{"version flag", []string{"--version"}, 0, "tess dev\n", ""},
		{"help", []string{"help"}, 0, "Usage: tess", ""},
		{"no args", nil, 2, "", "Usage: tess"},
		{"unknown", []string{"nope"}, 2, "", `unknown command "nope"`},
		{"serve bad port", []string{"serve", "--port", "0"}, 2, "", "out of range"},
		{"serve high port", []string{"serve", "--port", "70000"}, 2, "", "out of range"},
		{"serve extra arg", []string{"serve", "0.0.0.0"}, 2, "", "unexpected argument"},
		{"serve unknown flag", []string{"serve", "--addr", "0.0.0.0:7777"}, 2, "", "flag provided but not defined"},
		{"serve help", []string{"serve", "-h"}, 0, "", "-listen"},
		{"serve bad listen", []string{"serve", "--listen", "example.com"}, 2, "", "not an IP address"},
		{"serve listen with port", []string{"serve", "--listen", "0.0.0.0:7777"}, 2, "", "not an IP address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestServeStopsOnCancel(t *testing.T) {
	home := testHome(t)
	t.Setenv("TESSALY_HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Serve should start, see the cancelled context and shut down cleanly.

	var stdout, stderr bytes.Buffer
	// Port 0 is rejected, so pick a high port that is unlikely to be in use.
	code := run(ctx, []string{"serve", "--port", "47777"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "listening on http://127.0.0.1:47777") {
		t.Errorf("stdout = %q, want listen line", stdout.String())
	}

	// stdout is not a terminal, so the token must not be printed.
	token, err := os.ReadFile(filepath.Join(home, "token"))
	if err != nil {
		t.Fatalf("token file not created: %v", err)
	}
	if strings.Contains(stdout.String(), strings.TrimSpace(string(token))) {
		t.Error("stdout contains the token")
	}
	if !strings.Contains(stdout.String(), "Sign in with the token in "+filepath.Join(home, "token")) {
		t.Errorf("stdout = %q, want token path hint", stdout.String())
	}
}

func TestServeListenWarnsOffLoopback(t *testing.T) {
	t.Setenv("TESSALY_HOME", testHome(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr bytes.Buffer
	code := run(ctx, []string{"serve", "--listen", "0.0.0.0", "--port", "47779"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "listening on http://0.0.0.0:47779") {
		t.Errorf("stdout = %q, want listen line", stdout.String())
	}
	if !strings.Contains(stderr.String(), "only a container or a VPN address should do this") {
		t.Errorf("stderr = %q, want a warning", stderr.String())
	}
}

func TestServeRefusesOpenHome(t *testing.T) {
	home := t.TempDir()
	if err := os.Chmod(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TESSALY_HOME", home)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"serve", "--port", "47778"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "other users can reach it") {
		t.Errorf("code = %d, stderr = %q, want 1 and a mode error", code, stderr.String())
	}
}

func TestServeSignInHintFollowsPasskeys(t *testing.T) {
	home := testHome(t)
	t.Setenv("TESSALY_HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"serve", "--port", "47778"}, &stdout, &stderr); code != 0 {
		t.Fatalf("first serve = %d (%s)", code, stderr.String())
	}
	db, err := store.Open(context.Background(), filepath.Join(home, "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddPasskey(context.Background(), []byte{1}, "Laptop", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout.Reset()
	if code := run(ctx, []string{"serve", "--port", "47778"}, &stdout, &stderr); code != 0 {
		t.Fatalf("serve = %d (%s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Open http://localhost:47778 and sign in with your passkey") {
		t.Errorf("with a passkey, stdout = %q, want the passkey hint", stdout.String())
	}

	stdout.Reset()
	if code := run(ctx, []string{"serve", "--port", "47778", "--allow-token-login"}, &stdout, &stderr); code != 0 {
		t.Fatalf("serve --allow-token-login = %d (%s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sign in with the token in") {
		t.Errorf("with --allow-token-login, stdout = %q, want the token hint", stdout.String())
	}
}

// testHome returns a data folder path that does not exist yet. It sits
// under /tmp, because the control socket path must stay short and the
// macOS test folder is long.
func testHome(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "tess")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "home")
}

func TestServePrintsPublicURL(t *testing.T) {
	home := testHome(t)
	t.Setenv("TESSALY_HOME", home)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ url, want, notWant string }{
		{"https://tess.example.com", "Also open at https://tess.example.com", "Passkeys need https"},
		{"http://tess.tailnet:7777", "Passkeys need https, so they do not work at http://tess.tailnet:7777", ""},
	} {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(`public_url = "`+tt.url+`"`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var stdout, stderr bytes.Buffer
		if code := run(ctx, []string{"serve", "--port", "47779"}, &stdout, &stderr); code != 0 {
			t.Fatalf("serve = %d: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), tt.want) ||
			(tt.notWant != "" && strings.Contains(stdout.String(), tt.notWant)) {
			t.Errorf("public_url %s: stdout = %q, want %q", tt.url, stdout.String(), tt.want)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(`public_url = "tess.example.com"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"serve", "--port", "47779"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "public_url") {
		t.Errorf("bad public_url: code %d, stderr %q, want 1 and an error", code, stderr.String())
	}
}
