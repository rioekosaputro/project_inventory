-- 000009_init_tracking_tickets.up.sql (Fase 3 — T3.1)
-- Tracking pengiriman driver (GPS & POD) dan tiket maintenance teknisi.

CREATE TABLE shipment_tracking_logs (
    id               BIGSERIAL PRIMARY KEY,
    lpb_id           BIGINT NOT NULL REFERENCES lpb_documents(id),
    driver_id        BIGINT NOT NULL REFERENCES users(id),
    status           VARCHAR(20) NOT NULL CHECK (status IN (
                         'PICKED_UP','IN_TRANSIT','ARRIVED','DELIVERED','FAILED')),
    latitude         DECIMAL(10, 7) NOT NULL,
    longitude        DECIMAL(10, 7) NOT NULL,
    accuracy_m       INT,
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),   -- waktu server
    client_time      TIMESTAMPTZ,                          -- waktu HP (bisa offline)
    client_event_id  UUID UNIQUE,                          -- idempotensi retry dari HP
    receiver_name    VARCHAR(100),
    photo_path       TEXT,
    failure_reason   TEXT,
    notes            TEXT
);
CREATE INDEX idx_tracking_lpb ON shipment_tracking_logs(lpb_id, recorded_at);

CREATE TABLE maintenance_tickets (
    id                 BIGSERIAL PRIMARY KEY,
    ticket_number      VARCHAR(50) NOT NULL UNIQUE,
    customer_id        BIGINT NOT NULL REFERENCES customers(id),
    customer_site_id   BIGINT NOT NULL REFERENCES customer_sites(id),
    product_serial_id  BIGINT NOT NULL REFERENCES product_serials(id),
    reported_by_name   VARCHAR(100),
    reported_via       VARCHAR(20) NOT NULL DEFAULT 'PHONE' CHECK (reported_via IN (
                           'PHONE','WHATSAPP','EMAIL','PORTAL','INTERNAL')),
    category           VARCHAR(30) NOT NULL DEFAULT 'OTHER' CHECK (category IN (
                           'PAPER_JAM','PRINT_QUALITY','HARDWARE','SOFTWARE','NETWORK','PREVENTIVE','OTHER')),
    priority           VARCHAR(10) NOT NULL DEFAULT 'MEDIUM' CHECK (priority IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    status             VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN (
                           'OPEN','ASSIGNED','IN_PROGRESS','WAITING_PART','RESOLVED','CLOSED','CANCELLED')),
    title              VARCHAR(200) NOT NULL,
    description        TEXT,
    assigned_to        BIGINT REFERENCES users(id),
    sla_due_at         TIMESTAMPTZ,
    resolution_notes   TEXT,
    resolved_at        TIMESTAMPTZ,
    closed_at          TIMESTAMPTZ,
    swap_so_id         BIGINT REFERENCES sales_orders(id),     -- eskalasi UNIT_SWAP
    created_by         BIGINT NOT NULL REFERENCES users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tickets_status   ON maintenance_tickets(status, assigned_to);
CREATE INDEX idx_tickets_serial   ON maintenance_tickets(product_serial_id);

CREATE TABLE ticket_spareparts (
    id                 BIGSERIAL PRIMARY KEY,
    ticket_id          BIGINT NOT NULL REFERENCES maintenance_tickets(id) ON DELETE CASCADE,
    product_id         BIGINT NOT NULL REFERENCES products(id),
    qty                INT NOT NULL CHECK (qty > 0),
    status             VARCHAR(20) NOT NULL DEFAULT 'REQUESTED' CHECK (status IN (
                           'REQUESTED','LPB_CREATED','ISSUED','REJECTED','CANCELLED')),
    lpb_id             BIGINT REFERENCES lpb_documents(id),    -- LPB OUTBOUND SPARE_PART_ISSUE
    is_billable        BOOLEAN NOT NULL DEFAULT FALSE,         -- ditagihkan ke pelanggan?
    requested_by       BIGINT NOT NULL REFERENCES users(id),
    requested_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes              TEXT
);

CREATE TABLE ticket_activities (
    id             BIGSERIAL PRIMARY KEY,
    ticket_id      BIGINT NOT NULL REFERENCES maintenance_tickets(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    activity_type  VARCHAR(20) NOT NULL CHECK (activity_type IN (
                       'COMMENT','STATUS_CHANGE','ASSIGN','VISIT_CHECKIN','VISIT_CHECKOUT','PHOTO')),
    message        TEXT,
    from_status    VARCHAR(20),
    to_status      VARCHAR(20),
    latitude       DECIMAL(10, 7),
    longitude      DECIMAL(10, 7),
    photo_path     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ticket_activities_ticket ON ticket_activities(ticket_id, created_at);

ALTER TABLE sales_orders
    ADD CONSTRAINT fk_so_ticket FOREIGN KEY (related_ticket_id) REFERENCES maintenance_tickets(id);

CREATE TRIGGER trg_tickets_updated BEFORE UPDATE ON maintenance_tickets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
