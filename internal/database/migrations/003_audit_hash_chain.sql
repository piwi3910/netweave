-- 003_audit_hash_chain.sql
-- Add hash-chain columns to the audit_events table so the integrity of the
-- log can be verified after-the-fact. Each new entry stores:
--   - prev_hash:  the entry_hash of the previous event, or '' for genesis.
--   - entry_hash: SHA-256 of a canonical serialization of this event
--                 (including prev_hash), as computed by the application.
--
-- Combined with an append-only role and no UPDATE privileges at the DB layer,
-- this gives tamper-evidence: any mutation or deletion breaks the chain and
-- is detectable by the verification query.

ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS prev_hash  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS entry_hash TEXT NOT NULL DEFAULT '';

-- Index to walk the chain forwards/backwards efficiently during verification.
CREATE INDEX IF NOT EXISTS idx_audit_events_entry_hash ON audit_events (entry_hash) WHERE entry_hash != '';
CREATE INDEX IF NOT EXISTS idx_audit_events_prev_hash  ON audit_events (prev_hash)  WHERE prev_hash  != '';
