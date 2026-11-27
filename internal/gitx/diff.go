// SPDX-License-Identifier: Apache-2.0

package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// maxUntracked caps how many new files Diff shows, so a folder that should
// be ignored, such as node_modules, cannot make it run for long.
const maxUntracked = 500

// commitRE matches a full SHA-1 or SHA-256 commit hash.
var commitRE = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// diffFlags keep diff output plain. --no-ext-diff and --no-textconv stop
// repo config from running other programs to render the diff.
var diffFlags = []string{"-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff", "--no-textconv"}

// Diff returns the changes in the work tree at dir since the base commit as
// one unified diff: commits on the branch, staged and unstaged edits, and new
// files that git does not ignore. The diff stops at limit bytes, and
// truncated says so.
func Diff(ctx context.Context, dir, base string, limit int) (patch string, truncated bool, err error) {
	if !commitRE.MatchString(base) {
		return "", false, fmt.Errorf("base %q is not a commit hash", base)
	}
	out := &capped{max: limit}
	if err := diffTo(ctx, dir, out, append(diffFlags, "--find-renames", base, "--")...); err != nil {
		return "", false, err
	}

	list, err := run(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", false, err
	}
	files := strings.Split(list, "\x00")
	if files[len(files)-1] == "" {
		files = files[:len(files)-1]
	}
	if len(files) > maxUntracked {
		files = files[:maxUntracked]
		out.over = true
	}
	for _, file := range files {
		if err := diffTo(ctx, dir, out, append(diffFlags, "--no-index", "--", os.DevNull, file)...); err != nil {
			return "", false, err
		}
		if out.full() {
			break
		}
	}
	return out.buf.String(), out.over, nil
}

// diffTo runs a git diff in dir and writes its output to out. Exit code 1
// means "there are differences", which is not an error here.
func diffTo(ctx context.Context, dir string, out *capped, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", noHooks(args)...) // #nosec G204 -- fixed argv, no shell.
	cmd.Dir = dir
	cmd.Env = append(env(os.Environ()), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stdout = out
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err == nil || (errors.As(err, &exit) && exit.ExitCode() == 1) {
		return nil
	}
	return fmt.Errorf("git diff: %w: %s", err, strings.TrimSpace(stderr.String()))
}

// capped keeps the first max bytes written to it and drops the rest, so a
// huge diff cannot fill the daemon's memory.
type capped struct {
	buf  bytes.Buffer
	max  int
	over bool
}

// full reports whether the cap is reached.
func (c *capped) full() bool { return c.buf.Len() >= c.max }

func (c *capped) Write(p []byte) (int, error) {
	room := c.max - c.buf.Len()
	if len(p) > room {
		c.over = true
		if room > 0 {
			c.buf.Write(p[:room])
		}
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}
