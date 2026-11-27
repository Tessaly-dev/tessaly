// SPDX-License-Identifier: Apache-2.0

package gitx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// pushTimeout gives a push of a big branch over a slow link time to finish.
const pushTimeout = 2 * time.Minute

// Dirty reports whether the work tree at dir has uncommitted changes or new
// files that git does not ignore.
func Dirty(ctx context.Context, dir string) (bool, error) {
	out, err := run(ctx, dir, "status", "--porcelain")
	return out != "", err
}

// CommitAll stages every change in the work tree at dir and commits it with
// message, as the author from the user's git config. The message goes in on
// stdin, so it can never be read as a flag.
func CommitAll(ctx context.Context, dir, message string) error {
	if _, err := run(ctx, dir, "add", "--all"); err != nil {
		return err
	}
	_, err := runOpts(ctx, dir, opts{stdin: strings.NewReader(message)}, "commit", "--quiet", "--file=-")
	return err
}

// Push pushes branch from the repo at dir to the same name on remote and
// sets it as the upstream. It never forces.
func Push(ctx context.Context, dir, remote, branch string) error {
	ref := "refs/heads/" + branch
	_, err := runOpts(ctx, dir, opts{timeout: pushTimeout}, "push", "--quiet", "--set-upstream", "--", remote, ref+":"+ref)
	return err
}

// Ahead returns how many commits branch has that base does not. base must
// be a full commit hash.
func Ahead(ctx context.Context, dir, base, branch string) (int, error) {
	if !commitRE.MatchString(base) {
		return 0, fmt.Errorf("base %q is not a commit hash", base)
	}
	out, err := run(ctx, dir, "rev-list", "--count", base+"..refs/heads/"+branch, "--")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}
