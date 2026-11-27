// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(HomeEnv, dir+"/sub/../tessaly")
	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "tessaly"); got != want {
		t.Errorf("Home() = %q, want %q", got, want)
	}

	t.Setenv(HomeEnv, "relative/path")
	if _, err := Home(); err == nil {
		t.Error("Home() accepted a relative TESSALY_HOME")
	}

	t.Setenv(HomeEnv, "")
	t.Setenv("HOME", dir)
	got, err = Home()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".tessaly"); got != want {
		t.Errorf("Home() = %q, want %q", got, want)
	}
}

func TestEnsureHome(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "home")
	if err := EnsureHome(dir); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("mode = %04o, want 0700", perm)
	}

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureHome(dir); err == nil || !strings.Contains(err.Error(), "other users") {
		t.Errorf("EnsureHome on 0755 folder: err = %v, want refusal", err)
	}
}
