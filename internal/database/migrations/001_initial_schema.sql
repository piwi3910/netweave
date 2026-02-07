-- 001_initial_schema.sql
-- Initial PostgreSQL schema for netweave gateway.
-- Migrates persistent data from Redis to PostgreSQL.

-- Subscriptions table (migrated from Redis)
CREATE TABLE IF NOT EXISTS subscriptions (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT '',
    callback        TEXT NOT NULL,
    consumer_subscription_id TEXT NOT NULL DEFAULT '',
    filter_resource_pool_id  TEXT NOT NULL DEFAULT '',
    filter_resource_type_id  TEXT NOT NULL DEFAULT '',
    filter_resource_id       TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_tenant_id ON subscriptions (tenant_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_resource_pool ON subscriptions (filter_resource_pool_id) WHERE filter_resource_pool_id != '';
CREATE INDEX IF NOT EXISTS idx_subscriptions_resource_type ON subscriptions (filter_resource_type_id) WHERE filter_resource_type_id != '';

-- Tenants table (migrated from Redis)
CREATE TABLE IF NOT EXISTS tenants (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',
    quota           JSONB NOT NULL DEFAULT '{}',
    usage           JSONB NOT NULL DEFAULT '{}',
    contact_email   TEXT NOT NULL DEFAULT '',
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants (status);

-- Users table (migrated from Redis)
CREATE TABLE IF NOT EXISTS users (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    subject         TEXT NOT NULL DEFAULT '',
    common_name     TEXT NOT NULL DEFAULT '',
    email           TEXT NOT NULL DEFAULT '',
    oauth_subject   TEXT NOT NULL DEFAULT '',
    oauth_provider  TEXT NOT NULL DEFAULT '',
    role_id         TEXT NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users (tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_subject ON users (subject) WHERE subject != '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oauth_subject ON users (oauth_subject) WHERE oauth_subject != '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users (email) WHERE email != '';

-- Roles table (migrated from Redis)
CREATE TABLE IF NOT EXISTS roles (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL DEFAULT 'tenant',
    description     TEXT NOT NULL DEFAULT '',
    permissions     JSONB NOT NULL DEFAULT '[]',
    tenant_id       TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_name_tenant ON roles (name, tenant_id);

-- Audit events table (migrated from Redis)
CREATE TABLE IF NOT EXISTS audit_events (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    tenant_id       TEXT NOT NULL DEFAULT '',
    user_id         TEXT NOT NULL DEFAULT '',
    subject         TEXT NOT NULL DEFAULT '',
    resource_type   TEXT NOT NULL DEFAULT '',
    resource_id     TEXT NOT NULL DEFAULT '',
    action          TEXT NOT NULL DEFAULT '',
    details         JSONB NOT NULL DEFAULT '{}',
    client_ip       TEXT NOT NULL DEFAULT '',
    user_agent      TEXT NOT NULL DEFAULT '',
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_events_tenant_timestamp ON audit_events (tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_user_id ON audit_events (user_id) WHERE user_id != '';
CREATE INDEX IF NOT EXISTS idx_audit_events_type ON audit_events (type);

-- Notification deliveries table (migrated from Redis)
CREATE TABLE IF NOT EXISTS notification_deliveries (
    id              TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL,
    subscription_id TEXT NOT NULL,
    callback_url    TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 3,
    last_attempt_at TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ,
    last_error      TEXT NOT NULL DEFAULT '',
    http_status_code INTEGER NOT NULL DEFAULT 0,
    response_time   BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deliveries_event_id ON notification_deliveries (event_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_subscription_id ON notification_deliveries (subscription_id);
CREATE INDEX IF NOT EXISTS idx_deliveries_status ON notification_deliveries (status) WHERE status = 'failed';

-- Backend instances table (NEW: multi-backend support)
CREATE TABLE IF NOT EXISTS backend_instances (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    category        TEXT NOT NULL,  -- 'ims' or 'dms'
    adapter_type    TEXT NOT NULL,  -- e.g. 'kubernetes', 'helm', 'aws'
    description     TEXT NOT NULL DEFAULT '',
    config          BYTEA,          -- encrypted configuration blob
    credentials     BYTEA,          -- encrypted credentials blob
    status          TEXT NOT NULL DEFAULT 'disconnected',
    status_message  TEXT NOT NULL DEFAULT '',
    last_health_check TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_backends_category ON backend_instances (category);
CREATE UNIQUE INDEX IF NOT EXISTS idx_backends_name ON backend_instances (name);

-- Backend links table (NEW: IMS↔DMS many-to-many)
CREATE TABLE IF NOT EXISTS backend_links (
    ims_id          TEXT NOT NULL REFERENCES backend_instances(id) ON DELETE CASCADE,
    dms_id          TEXT NOT NULL REFERENCES backend_instances(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ims_id, dms_id)
);

-- Backend access table (NEW: tenant-scoped backend permissions)
CREATE TABLE IF NOT EXISTS backend_access (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    backend_id      TEXT NOT NULL REFERENCES backend_instances(id) ON DELETE CASCADE,
    permissions     JSONB NOT NULL DEFAULT '["read"]',
    constraints     JSONB NOT NULL DEFAULT '{}',
    granted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    granted_by      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_backend_access_tenant ON backend_access (tenant_id);
CREATE INDEX IF NOT EXISTS idx_backend_access_backend ON backend_access (backend_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_backend_access_tenant_backend ON backend_access (tenant_id, backend_id);
