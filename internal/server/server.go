// SPDX-License-Identifier: Apache-2.0

// Package server runs the Tessaly HTTP server. It serves a health endpoint,
// the embedded UI and the API under /api/v1/, and only ever listens on 127.0.0.1.
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
	"sync"
	"time"

	"github.com/tessaly-dev/tessaly/internal/auth"
	"github.com/tessaly-dev/tessaly/internal/store"
	"github.com/tessaly-dev/tessaly/internal/version"
	"github.com/tessaly-dev/tessaly/internal/workspace"
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

// Config holds what the HTTP handler needs.
type Config struct {
	// Port is the port the server listens on. The Host check uses it.
	Port int
	// UI holds the built web UI. It may be empty when the UI has not been built.
	UI fs.FS
	// Auth checks sign-in for the API.
	Auth *auth.Auth
	// Store holds repos and other state.
	Store *store.Store
	// Workspaces creates and archives workspaces.
	Workspaces *workspace.Manager
}

// Handler returns the HTTP handler for cfg.
func Handler(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.Handle("/", uiHandler(cfg.UI))

	// Sign-in and sign-out are the only API routes open without a session.
	mux.HandleFunc("POST /api/v1/login", cfg.Auth.Login)
	mux.HandleFunc("POST /api/v1/logout", cfg.Auth.Logout)

	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/session", session)
	repos := repoHandlers{store: cfg.Store, addMu: &sync.Mutex{}}
	api.HandleFunc("GET /api/v1/repos", repos.list)
	api.HandleFunc("POST /api/v1/repos", repos.add)
	api.HandleFunc("DELETE /api/v1/repos/{id}", repos.remove)
	ws := workspaceHandlers{store: cfg.Store, manager: cfg.Workspaces}
	api.HandleFunc("GET /api/v1/workspaces", ws.list)
	api.HandleFunc("POST /api/v1/workspaces", ws.create)
	api.HandleFunc("POST /api/v1/workspaces/{id}/archive", ws.archive)
	// Every other API path needs a session too, so unknown paths answer 401
	// to callers who are not signed in, and 404 to those who are.
	mux.Handle("/api/", cfg.Auth.Require(api))

	// CrossOriginProtection rejects state-changing requests that a browser
	// sends from another site, which stops CSRF against the local API.
	csrf := http.NewCrossOriginProtection()
	return securityHeaders(hostCheck(cfg.Port, csrf.Handler(mux)))
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

// session replies 200 to a signed-in caller. The UI uses it to decide whether
// to show the sign-in form.
func session(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"signedIn":true}` + "\n"))
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

// uiHandler serves files from ui. It indexes the files once at start, so a
// request can only ever reach a file that is in the bundle. Folders are never
// listed and dotfiles are never served.
func uiHandler(ui fs.FS) http.Handler {
	files := indexFiles(ui)
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

		file, ok := files[name]
		if !ok {
			if name == "index.html" {
				http.Error(w, "UI not built. Run: make ui", http.StatusServiceUnavailable)
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, ui, file)
	})
}

// indexFiles returns every regular file in ui, keyed by its path, leaving out
// anything under a dotfile or dot folder.
func indexFiles(ui fs.FS) map[string]string {
	files := make(map[string]string)
	_ = fs.WalkDir(ui, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable entries; the bundle is read-only.
		}
		if p != "." && hasDotSegment(p) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			files[p] = p
		}
		return nil
	})
	return files
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
