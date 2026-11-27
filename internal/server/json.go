// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// readJSON decodes a JSON request body into v. It needs a JSON content type,
// caps the body size and rejects unknown fields. On failure it writes a 4xx
// reply and returns false.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return readJSONMax(w, r, v, maxJSONBody)
}

// readJSONMax is readJSON with a body cap of limit bytes.
func readJSONMax(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "send Content-Type: application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError replies {"error": msg}. msg is shown to the user, so it must not
// carry internal details.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError logs err and replies 500 without the details.
func serverError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
