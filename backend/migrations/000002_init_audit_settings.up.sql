-- 000002_init_audit_settings.up.sql
-- Audit log (append-only), pengaturan aplikasi, lampiran, idempotency, job_runs.
-- Bergantung pada 000001 (users, forbid_update_delete).

CREATE TABLE audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT REFERENCES users(id),
    action       VARCHAR(60) NOT NULL,      -- LPB_APPROVE, USER_CREATE, INVOICE_VOID, LOGIN_FAILED, ...
    entity_type  VARCHAR(40),
    entity_id    BIGINT,
    before_data  JSONB,
    after_data   JSONB,
    ip_address   INET,
    user_agent   TEXT,
    request_id   VARCHAR(64),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_user   ON audit_logs(user_id, created_at);
CREATE TRIGGER trg_audit_append_only
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION forbid_update_delete();

CREATE TABLE app_settings (
    key         VARCHAR(80) PRIMARY KEY,   -- company.profile, lpb.allow_self_approve, billing.default_mode, ...
    value       JSONB NOT NULL,
    updated_by  BIGINT REFERENCES users(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE attachments (
    id           BIGSERIAL PRIMARY KEY,
    entity_type  VARCHAR(40) NOT NULL,
    entity_id    BIGINT NOT NULL,
    file_path    TEXT NOT NULL,
    file_name    VARCHAR(200),
    mime_type    VARCHAR(100) NOT NULL,
    size_bytes   BIGINT NOT NULL,
    uploaded_by  BIGINT REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_attachments_entity ON attachments(entity_type, entity_id);

-- Idempotency untuk endpoint kritis (approve LPB, deliver POD, record payment)
CREATE TABLE idempotency_keys (
    idem_key       VARCHAR(100) NOT NULL,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    endpoint       VARCHAR(150) NOT NULL,
    status_code    INT,
    response_body  JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (idem_key, user_id)
);

CREATE TABLE job_runs (
    id           BIGSERIAL PRIMARY KEY,
    job_name     VARCHAR(60) NOT NULL,      -- BILLING_MONTHLY, LOW_STOCK_ALERT, STOCK_RECONCILE, CONTRACT_EXPIRY
    run_key      VARCHAR(60) NOT NULL,      -- mis. 2026-11-21
    status       VARCHAR(20) NOT NULL CHECK (status IN ('RUNNING','SUCCESS','FAILED')),
    summary      JSONB,
    error        TEXT,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ,
    UNIQUE (job_name, run_key)
);
