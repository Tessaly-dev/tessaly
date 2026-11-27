// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// This file holds the sign-in settings rows and the audit log that
// tess auth reset writes.

// AuthEvent is one row of the sign-in audit log.
type AuthEvent struct {
	At       time.Time
	Kind     string // "reset"
	Method   string // "socket" when the daemon ran, "direct" when it did not
	Passkeys int64  // Passkeys removed
	Sessions int    // Sessions ended
}

// Settings rows that sign-in keeps.
const (
	// tokenState is "" for a reusable token, "reset:<RFC 3339 time>" for a
	// single-use token from a reset that works until that time, and "used"
	// once that token has signed in.
	tokenState = "token_state"
	// resetNotice holds the time of the last reset until the user dismisses
	// the notice in the UI.
	resetNotice = "reset_notice"
)

// TokenState returns the token state row, or "" when there is none.
func (s *Store) TokenState(ctx context.Context) (string, error) {
	return s.setting(ctx, tokenState)
}

// SetTokenState stores the token state. "" removes the row.
func (s *Store) SetTokenState(ctx context.Context, state string) error {
	return s.setSetting(ctx, tokenState, state)
}

// ResetNotice returns the time of the last reset the user has not
// dismissed, or the zero time.
func (s *Store) ResetNotice(ctx context.Context) (time.Time, error) {
	v, err := s.setting(ctx, resetNotice)
	if err != nil || v == "" {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, v)
}

// DismissResetNotice removes the reset notice.
func (s *Store) DismissResetNotice(ctx context.Context) error {
	return s.setSetting(ctx, resetNotice, "")
}

func (s *Store) setting(ctx context.Context, name string) (string, error) {
	var v []byte
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = ?", name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read setting %s: %w", name, err)
	}
	return string(v), nil
}

func (s *Store) setSetting(ctx context.Context, name, value string) error {
	var err error
	if value == "" {
		_, err = s.db.ExecContext(ctx, "DELETE FROM settings WHERE name = ?", name)
	} else {
		_, err = s.db.ExecContext(ctx,
			"INSERT INTO settings (name, value) VALUES (?, ?) ON CONFLICT (name) DO UPDATE SET value = excluded.value",
			name, []byte(value))
	}
	if err != nil {
		return fmt.Errorf("save setting %s: %w", name, err)
	}
	return nil
}

// ResetPending is the token state after a reset while no daemon ran: the
// token's time starts when tess serve next starts.
const ResetPending = "reset:pending"

// ResetTokenState returns the token state for a reset token that works
// until until. A zero until gives ResetPending.
func ResetTokenState(until time.Time) string {
	if until.IsZero() {
		return ResetPending
	}
	return "reset:" + until.UTC().Format(time.RFC3339)
}

// ResetPasskeys deletes every passkey, marks the token as a single-use
// reset token that works until until (or ResetPending for a zero until), sets the reset notice and writes an
// audit row, all in one transaction. It returns how many passkeys it
// deleted.
func (s *Store) ResetPasskeys(ctx context.Context, method string, sessions int, until time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("reset passkeys: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, "DELETE FROM passkeys")
	if err != nil {
		return 0, fmt.Errorf("reset passkeys: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reset passkeys: %w", err)
	}
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO auth_events (at, kind, method, passkeys, sessions) VALUES (?, 'reset', ?, ?, ?)",
		now, method, n, sessions); err != nil {
		return 0, fmt.Errorf("reset passkeys: %w", err)
	}
	for name, value := range map[string]string{tokenState: ResetTokenState(until), resetNotice: now} {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO settings (name, value) VALUES (?, ?) ON CONFLICT (name) DO UPDATE SET value = excluded.value",
			name, []byte(value)); err != nil {
			return 0, fmt.Errorf("reset passkeys: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("reset passkeys: %w", err)
	}
	return n, nil
}

// AuthEvents returns the audit log, newest first.
func (s *Store) AuthEvents(ctx context.Context) ([]AuthEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT at, kind, method, passkeys, sessions FROM auth_events ORDER BY id DESC")
	if err != nil {
		return nil, fmt.Errorf("list auth events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AuthEvent
	for rows.Next() {
		var e AuthEvent
		var at string
		if err := rows.Scan(&at, &e.Kind, &e.Method, &e.Passkeys, &e.Sessions); err != nil {
			return nil, fmt.Errorf("list auth events: %w", err)
		}
		if e.At, err = time.Parse(time.RFC3339, at); err != nil {
			return nil, fmt.Errorf("list auth events: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
