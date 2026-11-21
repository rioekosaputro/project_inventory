-- 000002_init_audit_settings.down.sql
-- Trigger append-only ikut terhapus bersama tabel audit_logs (DROP TABLE tidak memicu trigger baris).

DROP TABLE IF EXISTS job_runs;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS app_settings;
DROP TABLE IF EXISTS audit_logs;
