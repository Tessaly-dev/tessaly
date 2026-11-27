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
	"syscall"

	"github.com/tessaly-dev/tessaly/internal/server"
	"github.com/tessaly-dev/tessaly/internal/version"
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

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "%s listening on http://%s\n", version.String(), addr)
	if err := server.Serve(ctx, ln, server.Handler(web.Dist(), *port)); err != nil {
		fmt.Fprintf(stderr, "tess serve: %v\n", err)
		return 1
	}
	return 0
}
