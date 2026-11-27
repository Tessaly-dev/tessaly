// SPDX-License-Identifier: Apache-2.0

// Command tess is the Tessaly CLI and daemon.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tessaly-dev/tessaly/internal/auth"
	"github.com/tessaly-dev/tessaly/internal/config"
	"github.com/tessaly-dev/tessaly/internal/server"
	"github.com/tessaly-dev/tessaly/internal/store"
	"github.com/tessaly-dev/tessaly/internal/version"
	"github.com/tessaly-dev/tessaly/internal/workspace"
	"github.com/tessaly-dev/tessaly/web"
)

const usage = `Usage: tess <command> [flags]

Commands:
  serve     Run the Tessaly server on 127.0.0.1
  version   Print the version
  help      Show this help

Run "tess serve -h" for serve flags.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one tess command and returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version.String())
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	case "serve":
		return serve(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "tess: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func serve(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", server.DefaultPort, "port to listen on (always bound to 127.0.0.1)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "tess serve: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	addr, err := server.Addr(*port)
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 2
	}

	home, err := config.Home()
	if err == nil {
		err = config.EnsureHome(home)
	}
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	tokenPath := filepath.Join(home, "token")
	token, err := auth.LoadOrCreateToken(tokenPath)
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "%s listening on http://%s\n", version.String(), addr)
	printSignIn(stdout, addr, token, tokenPath)

	// Startup steps such as migrations run to the end even if Ctrl-C arrives
	// meanwhile.
	startCtx := context.WithoutCancel(ctx)
	db, err := store.Open(startCtx, filepath.Join(home, "tessaly.db"))
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	settings, err := config.Load(filepath.Join(home, "config.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}

	workspaces := workspace.NewManager(db, home)
	if err := workspaces.Recover(startCtx); err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}

	cfg := server.Config{
		Port:       *port,
		UI:         web.Dist(),
		Auth:       auth.New(token),
		Store:      db,
		Workspaces: workspaces,
		Agents:     settings.Agents,
	}
	// Agents must not outlive the daemon. Stop them once serving ends,
	// whether by Ctrl-C or an error.
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), workspace.StopGrace+5*time.Second)
		defer cancel()
		workspaces.StopAll(stopCtx)
	}()
	if err := server.Serve(ctx, ln, server.Handler(cfg)); err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	return 0
}

// printSignIn tells the user how to sign in. On a terminal it prints a link
// that carries the token in the URL fragment, which the browser never sends
// to the server. Anywhere else, such as a service log, it prints only the
// token file path, so the token does not end up in log files.
func printSignIn(w io.Writer, addr, token, tokenPath string) {
	if isTerminal(w) {
		fmt.Fprintf(w, "Open http://%s/#token=%s to sign in\n", addr, token)
		return
	}
	fmt.Fprintf(w, "Sign in with the token in %s\n", tokenPath)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
