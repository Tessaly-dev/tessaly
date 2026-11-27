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
	"strconv"
	"syscall"
	"time"

	"github.com/tessaly-dev/tessaly/internal/auth"
	"github.com/tessaly-dev/tessaly/internal/checks"
	"github.com/tessaly-dev/tessaly/internal/config"
	"github.com/tessaly-dev/tessaly/internal/control"
	"github.com/tessaly-dev/tessaly/internal/forge"
	"github.com/tessaly-dev/tessaly/internal/server"
	"github.com/tessaly-dev/tessaly/internal/store"
	"github.com/tessaly-dev/tessaly/internal/version"
	"github.com/tessaly-dev/tessaly/internal/workspace"
	"github.com/tessaly-dev/tessaly/web"
)

const usage = `Usage: tess <command> [flags]

Commands:
  serve        Run the Tessaly server on 127.0.0.1
  auth reset   Remove every passkey, end every session and make a new token
  version      Print the version
  help         Show this help

Run "tess serve -h" for serve flags.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one tess command and returns the process exit code.
// stdin is where tess reads answers. Tests replace it.
var stdin io.Reader = os.Stdin

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
	case "auth":
		if len(args) < 2 || args[1] != "reset" {
			fmt.Fprint(stderr, "Usage: tess auth reset [--yes]\n")
			return 2
		}
		return authReset(ctx, args[2:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "tess: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func serve(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", server.DefaultPort, "port to listen on")
	listen := fs.String("listen", server.DefaultListen,
		"IP address to listen on. Keep 127.0.0.1 unless tess runs in a container")
	allowToken := fs.Bool("allow-token-login", false,
		"keep token sign-in on after you register a passkey")
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

	addr, err := server.Addr(*listen, *port)
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 2
	}
	// The browser always reaches tess on this machine's loopback, because the
	// Host check accepts nothing else. In a container, that is the published
	// port. The link uses localhost, the only host name passkeys work on.
	local := net.JoinHostPort(auth.PasskeyHost, strconv.Itoa(*port))

	home, err := config.Home()
	if err == nil {
		err = config.EnsureHome(home)
	}
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	// One daemon per data folder. The lock also tells "tess auth reset"
	// whether to ask this daemon or change the files itself.
	unlock, err := control.Lock(home)
	if errors.Is(err, control.ErrLocked) {
		fmt.Fprintf(stderr, "tess serve: another tess serve already uses %s\n", home)
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	defer unlock()

	settings, err := config.Load(filepath.Join(home, "config.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	public := settings.Public()

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

	// Startup steps such as migrations run to the end even if Ctrl-C arrives
	// meanwhile.
	startCtx := context.WithoutCancel(ctx)
	db, err := store.Open(startCtx, filepath.Join(home, "tessaly.db"))
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	signIn := auth.New(token)
	if err := signIn.EnablePasskeys(db, *port, public, *allowToken); err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	hasPasskeys, err := db.HasPasskeys(startCtx)
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "%s listening on http://%s\n", version.String(), addr)
	if !server.IsLoopback(*listen) {
		fmt.Fprintf(stderr, "tess serve: listening on %s, not only 127.0.0.1. "+
			"Requests must still use http://%s or public_url, and only a container or a VPN address should do this.\n",
			*listen, local)
	}
	if public != nil {
		fmt.Fprintf(stdout, "Also open at %s\n", public)
		if public.Scheme != "https" {
			fmt.Fprintf(stdout, "Passkeys need https, so they do not work at %s. Sign in there with the token "+
				"while token sign-in is on.\n", public)
		}
	}
	if hasPasskeys && !*allowToken {
		fmt.Fprintf(stdout, "Open http://%s and sign in with your passkey\n", local)
	} else {
		printSignIn(stdout, local, token, tokenPath)
	}

	workspaces := workspace.NewManager(db, home)
	if err := workspaces.Recover(startCtx); err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}

	ctl, err := control.Listen(home)
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	go control.Serve(ctx, ctl, resetHandler(workspaces.AgentPIDs, func(ctx context.Context) (any, error) {
		return resetRunning(ctx, signIn, db, tokenPath, local)
	}))

	github := forge.GitHub{}
	watcher := checks.New(db, github, checks.DefaultEvery)
	go watcher.Run(ctx)

	cfg := server.Config{
		Port:       *port,
		PublicURL:  public,
		UI:         web.Dist(),
		Auth:       signIn,
		Store:      db,
		Workspaces: workspaces,
		Agents:     settings.Agents,
		Forge:      github,
		Checks:     watcher,
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
