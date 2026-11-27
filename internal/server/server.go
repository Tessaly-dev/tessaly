// SPDX-License-Identifier: Apache-2.0

// Package server runs the Tessaly HTTP server. In this skeleton it serves a
// health endpoint and the embedded UI, and only ever listens on 127.0.0.1.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tessaly-dev/tessaly/internal/version"
)

// DefaultPort is the port "tess serve" uses when none is given.
const DefaultPort = 7777

// loopback is the only address Tessaly binds to. Remote access goes through an
// SSH tunnel, a VPN or a reverse proxy, never through a wider bind.
const loopback = "127.0.0.1"

// Addr returns the listen address for port, or an error if the port is out of range.
func Addr(port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port %d out of range 1-65535", port)
	}
	return net.JoinHostPort(loopback, strconv.Itoa(port)), nil
}

// Handler returns the HTTP handler for a server listening on port.
// ui holds the built web UI. It may be empty when the UI has not been built.
func Handler(ui fs.FS, port int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.Handle("/", uiHandler(ui))
	return securityHeaders(hostCheck(port, mux))
}

// Serve serves h on ln until ctx ends, then shuts down within 5 seconds.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Version: version.Version})
}

// uiHandler serves files from ui. It never lists folders and hides dotfiles.
func uiHandler(ui fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if hasDotSegment(name) {
			http.NotFound(w, r)
			return
		}

		info, err := fs.Stat(ui, name)
		if err != nil || info.IsDir() {
			if name == "index.html" {
				http.Error(w, "UI not built. Run: make ui", http.StatusServiceUnavailable)
				return
			}
			http.NotFound(w, r)
			return
		}

		http.ServeFileFS(w, r, ui, name)
	})
}

func hasDotSegment(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// hostCheck rejects requests whose Host header is not this server. This blocks
// DNS rebinding, where a web page on another site resolves its own name to
// 127.0.0.1 to reach the local API.
func hostCheck(port int, next http.Handler) http.Handler {
	p := strconv.Itoa(port)
	allowed := map[string]bool{
		net.JoinHostPort(loopback, p):    true,
		net.JoinHostPort("localhost", p): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[strings.ToLower(r.Host)] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
