-- SPDX-License-Identifier: Apache-2.0
-- Passkeys registered for sign-in. data is the WebAuthn credential as JSON:
-- the credential ID, public key and sign counter. It holds no secret.
CREATE TABLE passkeys (
    id            INTEGER PRIMARY KEY,
    credential_id BLOB NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    data          TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    last_used_at  TEXT NOT NULL DEFAULT ''
);

-- Values Tessaly makes once, such as the random WebAuthn user ID.
CREATE TABLE settings (
    name  TEXT PRIMARY KEY,
    value BLOB NOT NULL
);
