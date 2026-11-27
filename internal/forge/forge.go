// SPDX-License-Identifier: Apache-2.0

// Package forge opens pull requests on a code host. GitHub works through the
// gh CLI, so Tessaly reuses your gh sign-in and never stores or sees a token.
package forge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/tessaly-dev/tessaly/internal/gitx"
)

// Forge opens pull requests on a code host.
type Forge interface {
	// Checks returns the CI checks on the pull request at prURL.
	Checks(ctx context.Context, dir, prURL string) ([]Check, error)
	// OpenPR opens a pull request from branch, which must already be
	// pushed, into the repo's default branch. dir is a work tree of the
	// repo. It returns the pull request's https URL. When one is already
	// open for branch, it returns that one's URL.
	OpenPR(ctx context.Context, dir, branch, title, body string) (string, error)
}

// ErrNoCLI means the code host's CLI is not installed.
var ErrNoCLI = errors.New("the GitHub CLI (gh) is not installed. Install it and run gh auth login on this machine")

// timeout caps every gh call.
const timeout = time.Minute

// GitHub opens pull requests with the gh CLI.
type GitHub struct {
	// Bin is the gh program. Empty means "gh" from PATH.
	Bin string
}

// OpenPR runs gh pr create. The title goes in as one --title= argument and
// the body on stdin, so neither can be read as a flag.
func (g GitHub) OpenPR(ctx context.Context, dir, branch, title, body string) (string, error) {
	out, err := g.run(ctx, dir, body, "pr", "create", "--head="+branch, "--title="+title, "--body-file=-")
	if err != nil && strings.Contains(err.Error(), "already exists") {
		// The branch already has an open pull request, perhaps opened
		// outside Tessaly. Use it.
		out, err = g.run(ctx, dir, "", "pr", "view", "--json=url", "--jq=.url", "--", branch)
	}
	if err != nil {
		return "", err
	}
	lines := strings.Split(out, "\n")
	return checkURL(strings.TrimSpace(lines[len(lines)-1]))
}

// run runs gh with args in dir, with stdin, and returns trimmed stdout. When
// gh fails, it returns stdout as well as the error.
func (g GitHub) run(ctx context.Context, dir, stdin string, args ...string) (string, error) {
	bin := g.Bin
	if bin == "" {
		bin = "gh"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", ErrNoCLI
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...) // #nosec G204 -- fixed argv, no shell.
	cmd.Dir = dir
	cmd.Env = append(withoutGit(os.Environ()),
		"GH_PROMPT_DISABLED=1",    // Fail instead of asking a question.
		"GH_NO_UPDATE_NOTIFIER=1", // No network call to check for updates.
		"GH_SPINNER_DISABLED=1",   // Plain output.
		"NO_COLOR=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	// A new session has no controlling terminal, so nothing can prompt
	// in tess serve's terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), fmt.Errorf("gh %s %s: %s", args[0], args[1], gitx.Redact(strings.TrimSpace(stderr.String())))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// checkURL accepts only an absolute https URL, so the UI can link to it
// without risk of a javascript: or other scheme.
func checkURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("gh returned %q, not a pull request URL", raw)
	}
	return u.String(), nil
}

// withoutGit drops GIT_* variables, which could point gh's git calls at
// another repo.
func withoutGit(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if !strings.HasPrefix(kv, "GIT_") {
			out = append(out, kv)
		}
	}
	return out
}
