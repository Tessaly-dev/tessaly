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
	yes := fs.Bool("yes", false, "do not ask before resetting")
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

	if !*yes && !confirm(stdin, stdout) {
		fmt.Fprintln(stderr, "tess auth reset: not confirmed, nothing changed. Type reset, or pass --yes.")
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

// confirm asks the user to type "reset". Without a terminal it refuses, so
// a script must say --yes.
func confirm(stdin io.Reader, stdout io.Writer) bool {
	if f, ok := stdin.(*os.File); !ok || !isTerminal(f) {
		return false
	}
	fmt.Fprint(stdout, "This removes every passkey, signs everyone out and makes a new login token.\nType reset to continue: ")
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	return strings.TrimSpace(line) == "reset"
}

// resetRunning is the daemon's side of a reset: a new token, every session
// and passkey gone, and an audit row.
func resetRunning(ctx context.Context, a *auth.Auth, db *store.Store, tokenPath, addr string) (resetResult, error) {
	token, err := auth.ReplaceToken(tokenPath)
	if err != nil {
		return resetResult{}, err
	}
	sessions := a.Reset(token)
	passkeys, err := db.ResetPasskeys(ctx, "socket", sessions)
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
	passkeys, err := db.ResetPasskeys(ctx, "direct", 0)
	return resetResult{Passkeys: passkeys}, err
}
