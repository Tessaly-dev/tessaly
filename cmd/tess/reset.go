// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tessaly-dev/tessaly/internal/auth"
	"github.com/tessaly-dev/tessaly/internal/config"
	"github.com/tessaly-dev/tessaly/internal/control"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// resetResult is what a reset did. Token is set only when the daemon ran.
type resetResult struct {
	Passkeys int64  `json:"passkeys"`
	Sessions int    `json:"sessions"`
	Token    string `json:"token"`
	Addr     string `json:"addr"`
}

// authReset runs "tess auth reset". It works only on the host: there is no
// HTTP route for it. While tess serve runs it asks the daemon over the
// control socket, otherwise it changes the data folder itself.
func authReset(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("auth reset", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "tess auth reset: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	home, err := config.Home()
	if err == nil {
		err = config.CheckPrivate(home)
	}
	if err != nil {
		fmt.Fprintf(stderr, "tess auth reset: %v\n", err)
		return 1
	}

	host, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(stderr, "tess auth reset: %v\n", err)
		return 1
	}
	if !confirm(stdin, stdout, host) {
		fmt.Fprintf(stderr, "tess auth reset: not confirmed, nothing changed. Type %s to confirm.\n", host)
		return 1
	}

	tokenPath := filepath.Join(home, "token")
	var res resetResult
	unlock, err := control.Lock(home)
	switch {
	case errors.Is(err, control.ErrLocked):
		err = control.Call(home, control.CmdAuthReset, &res)
	case err == nil:
		res, err = resetStopped(ctx, home, tokenPath)
		unlock()
	}
	if err != nil {
		fmt.Fprintf(stderr, "tess auth reset: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Passkeys removed: %d. Sessions ended: %d. The old token no longer works.\n",
		res.Passkeys, res.Sessions)
	if res.Token == "" {
		fmt.Fprintln(stdout, "Start tess serve to sign in with the new token.")
		return 0
	}
	printSignIn(stdout, res.Addr, res.Token, tokenPath)
	return 0
}

// confirm asks the user to type this machine's host name, so a reset
// happens only on purpose and on the machine they meant. A script can pipe
// the name in.
func confirm(stdin io.Reader, stdout io.Writer, host string) bool {
	fmt.Fprintf(stdout, "This removes every passkey, signs everyone out and makes a login token that works once, for %s.\n"+
		"Type this machine's host name (%s) to continue: ", auth.ResetTokenTTL, host)
	line, _ := bufio.NewReader(io.LimitReader(stdin, 256)).ReadString('\n')
	return strings.TrimSpace(line) == host
}

// resetHandler answers the control socket. It refuses a caller that runs
// under an agent: an agent runs as the same user, so the UID check alone
// would let it reset sign-in and read the new token.
func resetHandler(agents func() []int, reset func(context.Context) (any, error)) control.Handler {
	return func(ctx context.Context, _ string, pid int) (any, error) {
		if control.InTree(pid, agents()) {
			slog.Warn("refused tess auth reset from an agent", "pid", pid)
			return nil, control.Refused("tess auth reset cannot run from an agent's terminal")
		}
		return reset(ctx)
	}
}

// resetRunning is the daemon's side of a reset: a new token, every session
// and passkey gone, and an audit row.
func resetRunning(ctx context.Context, a *auth.Auth, db *store.Store, tokenPath, addr string) (resetResult, error) {
	token, err := auth.ReplaceToken(tokenPath)
	if err != nil {
		return resetResult{}, err
	}
	until := time.Now().Add(auth.ResetTokenTTL)
	sessions := a.Reset(token, until)
	passkeys, err := db.ResetPasskeys(ctx, "socket", sessions, until)
	if err != nil {
		return resetResult{}, err
	}
	slog.Warn("sign-in was reset from the host", "passkeys", passkeys, "sessions", sessions)
	return resetResult{Passkeys: passkeys, Sessions: sessions, Token: token, Addr: addr}, nil
}

// resetStopped resets while no daemon runs. Sessions live only in a
// running daemon, so there are none to end.
func resetStopped(ctx context.Context, home, tokenPath string) (resetResult, error) {
	if _, err := auth.ReplaceToken(tokenPath); err != nil {
		return resetResult{}, err
	}
	db, err := store.Open(ctx, filepath.Join(home, "tessaly.db"))
	if err != nil {
		return resetResult{}, err
	}
	defer func() { _ = db.Close() }()
	// The 15 minutes start when tess serve starts, since nobody can use the
	// token before that. EnablePasskeys reads this mark.
	passkeys, err := db.ResetPasskeys(ctx, "direct", 0, time.Time{})
	return resetResult{Passkeys: passkeys}, err
}
