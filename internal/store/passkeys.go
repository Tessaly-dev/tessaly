// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Passkey is a registered WebAuthn credential. Data is the credential as
// JSON. It holds a public key and a counter, never a secret.
type Passkey struct {
	ID           int64
	CredentialID []byte
	Name         string
	Data         []byte
	CreatedAt    time.Time
	LastUsedAt   time.Time // Zero until the passkey first signs in.
}

// AddPasskey stores a passkey. It returns ErrExists if the credential ID is
// already registered.
func (s *Store) AddPasskey(ctx context.Context, credentialID []byte, name string, data []byte) (Passkey, error) {
	created := time.Now().UTC().Truncate(time.Second)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO passkeys (credential_id, name, data, created_at) VALUES (?, ?, ?, ?)",
		credentialID, name, string(data), created.Format(time.RFC3339))
	if err != nil {
		if isUniqueViolation(err) {
			return Passkey{}, ErrExists
		}
		return Passkey{}, fmt.Errorf("add passkey: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Passkey{}, fmt.Errorf("add passkey: %w", err)
	}
	return Passkey{ID: id, CredentialID: credentialID, Name: name, Data: data, CreatedAt: created}, nil
}

// Passkeys returns every passkey, oldest first.
func (s *Store) Passkeys(ctx context.Context) ([]Passkey, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, credential_id, name, data, created_at, last_used_at FROM passkeys ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list passkeys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	keys := []Passkey{}
	for rows.Next() {
		var p Passkey
		var data, created, used string
		if err := rows.Scan(&p.ID, &p.CredentialID, &p.Name, &data, &created, &used); err != nil {
			return nil, fmt.Errorf("list passkeys: %w", err)
		}
		p.Data = []byte(data)
		if p.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
			return nil, fmt.Errorf("list passkeys: %w", err)
		}
		if used != "" {
			if p.LastUsedAt, err = time.Parse(time.RFC3339, used); err != nil {
				return nil, fmt.Errorf("list passkeys: %w", err)
			}
		}
		keys = append(keys, p)
	}
	return keys, rows.Err()
}

// HasPasskeys reports whether any passkey is registered.
func (s *Store) HasPasskeys(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM passkeys").Scan(&n); err != nil {
		return false, fmt.Errorf("count passkeys: %w", err)
	}
	return n > 0, nil
}

// UsePasskey saves a passkey's credential data after a sign-in, which moves
// its counter on, and records the time.
func (s *Store) UsePasskey(ctx context.Context, id int64, data []byte) error {
	res, err := s.db.ExecContext(ctx, "UPDATE passkeys SET data = ?, last_used_at = ? WHERE id = ?",
		string(data), time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update passkey: %w", err)
	}
	return oneRow(res)
}

// DeletePasskey removes a passkey, or returns ErrNotFound.
func (s *Store) DeletePasskey(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM passkeys WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete passkey: %w", err)
	}
	return oneRow(res)
}

func oneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// webAuthnUserID is the settings row that holds the WebAuthn user ID.
const webAuthnUserID = "webauthn_user_id"

// WebAuthnUserID returns this install's random 32-byte WebAuthn user ID,
// making it on first use. Browsers keep one passkey per site and user ID, so
// a random ID stops two installs on the same host name from replacing each
// other's passkeys.
func (s *Store) WebAuthnUserID(ctx context.Context) ([]byte, error) {
	id := make([]byte, 32)
	_, _ = rand.Read(id) // crypto/rand.Read never fails on supported platforms.
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO settings (name, value) VALUES (?, ?) ON CONFLICT (name) DO NOTHING", webAuthnUserID, id); err != nil {
		return nil, fmt.Errorf("save webauthn user id: %w", err)
	}
	var got []byte
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = ?", webAuthnUserID).Scan(&got)
	if err != nil {
		return nil, fmt.Errorf("read webauthn user id: %w", err)
	}
	if len(got) != 32 {
		return nil, errors.New("webauthn user id is malformed")
	}
	return got, nil
}

// AuthEvent is one row of the sign-in audit log.
type AuthEvent struct {
	At       time.Time
	Kind     string // "reset"
	Method   string // "socket" when the daemon ran, "direct" when it did not
	Passkeys int64  // Passkeys removed
	Sessions int    // Sessions ended
}

// ResetPasskeys deletes every passkey and writes an audit row, in one
// transaction. It returns how many passkeys it deleted.
func (s *Store) ResetPasskeys(ctx context.Context, method string, sessions int) (int64, error) {
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
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO auth_events (at, kind, method, passkeys, sessions) VALUES (?, 'reset', ?, ?, ?)",
		time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), method, n, sessions); err != nil {
		return 0, fmt.Errorf("reset passkeys: %w", err)
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
