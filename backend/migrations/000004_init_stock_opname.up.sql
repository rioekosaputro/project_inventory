-- 000004_init_stock_opname.up.sql

CREATE TABLE stock_opnames (
    id                     BIGSERIAL PRIMARY KEY,
    opname_number          VARCHAR(50) NOT NULL UNIQUE,
    warehouse_id           BIGINT NOT NULL REFERENCES warehouses(id),
    status                 VARCHAR(20) NOT NULL DEFAULT 'IN_PROGRESS' CHECK (status IN (
                               'IN_PROGRESS','SUBMITTED','FINALIZED','CANCELLED')),
    is_blind_count         BOOLEAN NOT NULL DEFAULT TRUE,   -- staf tidak melihat qty sistem saat hitung
    started_by             BIGINT NOT NULL REFERENCES users(id),
    started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_by           BIGINT REFERENCES users(id),
    submitted_at           TIMESTAMPTZ,
    finalized_by           BIGINT REFERENCES users(id),
    finalized_at           TIMESTAMPTZ,
    adjustment_in_lpb_id   BIGINT REFERENCES lpb_documents(id),
    adjustment_out_lpb_id  BIGINT REFERENCES lpb_documents(id),
    notes                  TEXT
);

CREATE TABLE stock_opname_racks (
    opname_id  BIGINT NOT NULL REFERENCES stock_opnames(id) ON DELETE CASCADE,
    rack_id    BIGINT NOT NULL REFERENCES racks(id),
    PRIMARY KEY (opname_id, rack_id)
);

CREATE TABLE stock_opname_items (
    id            BIGSERIAL PRIMARY KEY,
    opname_id     BIGINT NOT NULL REFERENCES stock_opnames(id) ON DELETE CASCADE,
    product_id    BIGINT NOT NULL REFERENCES products(id),
    rack_id       BIGINT NOT NULL REFERENCES racks(id),
    system_qty    INT NOT NULL,          -- snapshot saat opname dimulai
    counted_qty   INT CHECK (counted_qty >= 0),
    diff_qty      INT GENERATED ALWAYS AS (counted_qty - system_qty) STORED,
    counted_by    BIGINT REFERENCES users(id),
    counted_at    TIMESTAMPTZ,
    notes         TEXT,
    UNIQUE (opname_id, product_id, rack_id)
);

CREATE TABLE stock_opname_serials (
    id                 BIGSERIAL PRIMARY KEY,
    opname_id          BIGINT NOT NULL REFERENCES stock_opnames(id) ON DELETE CASCADE,
    rack_id            BIGINT NOT NULL REFERENCES racks(id),
    product_serial_id  BIGINT REFERENCES product_serials(id),
    scanned_code       VARCHAR(100),
    result             VARCHAR(20) NOT NULL CHECK (result IN ('FOUND','MISSING','UNEXPECTED','WRONG_RACK')),
    scanned_by         BIGINT REFERENCES users(id),
    scanned_at         TIMESTAMPTZ,
    UNIQUE (opname_id, product_serial_id)
);

ALTER TABLE racks
    ADD CONSTRAINT fk_racks_locked_opname FOREIGN KEY (locked_by_opname_id) REFERENCES stock_opnames(id);
