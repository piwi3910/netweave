-- 002_certificate_metadata.sql
-- Certificate metadata table for lifecycle automation.
-- Tracks cert issuance, renewal, expiry, and revocation state.

CREATE TABLE IF NOT EXISTS certificate_metadata (
    serial_number   TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL DEFAULT '',
    tenant_id       TEXT NOT NULL DEFAULT '',
    common_name     TEXT NOT NULL DEFAULT '',
    role_name       TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    renewed_at      TIMESTAMPTZ,
    renewed_from    TEXT NOT NULL DEFAULT '',
    renewed_to      TEXT NOT NULL DEFAULT '',
    renewal_count   INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    next_retry_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cert_meta_tenant ON certificate_metadata (tenant_id);
CREATE INDEX IF NOT EXISTS idx_cert_meta_user ON certificate_metadata (user_id) WHERE user_id != '';
CREATE INDEX IF NOT EXISTS idx_cert_meta_status ON certificate_metadata (status);
CREATE INDEX IF NOT EXISTS idx_cert_meta_expires ON certificate_metadata (expires_at);
CREATE INDEX IF NOT EXISTS idx_cert_meta_active_expires ON certificate_metadata (status, expires_at) WHERE status = 'active';
