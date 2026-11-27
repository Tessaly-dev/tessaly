// SPDX-License-Identifier: Apache-2.0

package forge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGH writes a stand-in gh that saves its arguments and stdin next to
// itself and then runs script.
func fakeGH(t *testing.T, script string) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(dir, "gh")
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> \"" + dir + "/args\"\n" +
		"cat > \"" + dir + "/stdin\"\n" +
		"env > \"" + dir + "/env\"\n" +
		script + "\n"
	if err := os.WriteFile(bin, []byte(body), 0o700); err != nil { // #nosec G306 -- the test program must run.
		t.Fatal(err)
	}
	return bin, dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- test file.
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOpenPR(t *testing.T) {
	bin, dir := fakeGH(t, `echo "Creating pull request" >&2; echo https://github.com/acme/app/pull/7`)
	t.Setenv("GIT_DIR", "/elsewhere")

	url, err := GitHub{Bin: bin}.OpenPR(context.Background(), t.TempDir(), "tess/fix", "--draft; rm -rf ~", "Body\n--web")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/acme/app/pull/7" {
		t.Errorf("url = %q", url)
	}
	wantArgs := "pr\ncreate\n--head=tess/fix\n--title=--draft; rm -rf ~\n--body-file=-\n"
	if got := read(t, filepath.Join(dir, "args")); got != wantArgs {
		t.Errorf("args = %q, want %q", got, wantArgs)
	}
	if got := read(t, filepath.Join(dir, "stdin")); got != "Body\n--web" {
		t.Errorf("stdin = %q", got)
	}
	env := read(t, filepath.Join(dir, "env"))
	if !strings.Contains(env, "GH_PROMPT_DISABLED=1") || strings.Contains(env, "GIT_DIR=") {
		t.Errorf("env not cleaned:\n%s", env)
	}
}

func TestOpenPRUsesExistingPR(t *testing.T) {
	bin, dir := fakeGH(t, `if [ "$2" = create ]; then
  echo 'a pull request for branch "tess/fix" into branch "main" already exists:' >&2
  echo https://github.com/acme/app/pull/3 >&2
  exit 1
fi
echo https://github.com/acme/app/pull/3`)
	url, err := GitHub{Bin: bin}.OpenPR(context.Background(), t.TempDir(), "tess/fix", "t", "")
	if err != nil || url != "https://github.com/acme/app/pull/3" {
		t.Fatalf("OpenPR = %q, %v", url, err)
	}
	if args := read(t, filepath.Join(dir, "args")); !strings.HasSuffix(args, "pr\nview\n--json=url\n--jq=.url\n--\ntess/fix\n") {
		t.Errorf("args = %q", args)
	}
}

func TestOpenPRErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := (GitHub{Bin: filepath.Join(t.TempDir(), "no-gh")}).OpenPR(ctx, t.TempDir(), "b", "t", ""); !errors.Is(err, ErrNoCLI) {
		t.Errorf("missing gh: err = %v, want ErrNoCLI", err)
	}

	bin, _ := fakeGH(t, `echo "fatal: https://me:ghp_secret@github.com/a/b denied" >&2; exit 1`)
	_, err := GitHub{Bin: bin}.OpenPR(ctx, t.TempDir(), "b", "t", "")
	if err == nil || strings.Contains(err.Error(), "ghp_secret") || !strings.Contains(err.Error(), "denied") {
		t.Errorf("failing gh: err = %v, want the redacted message", err)
	}

	for _, out := range []string{"javascript:alert(1)", "http://github.com/a/b/pull/1", "https://u:p@github.com/x", "not a url"} {
		bin, _ := fakeGH(t, "echo '"+out+"'")
		if url, err := (GitHub{Bin: bin}).OpenPR(ctx, t.TempDir(), "b", "t", ""); err == nil {
			t.Errorf("gh printed %q: url = %q, want an error", out, url)
		}
	}
}
