-- 000003_init_inventory_lpb.up.sql
-- Stok hybrid, serial number, dokumen LPB, buku besar & riwayat SN, penomoran dokumen

CREATE TABLE document_sequences (
    doc_type     VARCHAR(20) NOT NULL,   -- LPBI, LPBO, SO, DO, INV, OPN, TKT, KTR, AST
    scope_key    VARCHAR(40) NOT NULL,   -- kode gudang atau 'GLOBAL'
    period       CHAR(6)     NOT NULL,   -- YYYYMM (WIB); '000000' utk sequence tanpa reset
    last_number  INT NOT NULL DEFAULT 0,
    PRIMARY KEY (doc_type, scope_key, period)
);

CREATE TABLE stock_balances (
    id            BIGSERIAL PRIMARY KEY,
    product_id    BIGINT NOT NULL REFERENCES products(id),
    rack_id       BIGINT NOT NULL REFERENCES racks(id),
    qty_on_hand   INT NOT NULL DEFAULT 0 CHECK (qty_on_hand >= 0),
    qty_reserved  INT NOT NULL DEFAULT 0 CHECK (qty_reserved >= 0 AND qty_reserved <= qty_on_hand),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, rack_id)
);
CREATE INDEX idx_stock_balances_rack ON stock_balances(rack_id);

CREATE TABLE product_serials (
    id                   BIGSERIAL PRIMARY KEY,
    product_id           BIGINT NOT NULL REFERENCES products(id),
    asset_tag            VARCHAR(30) NOT NULL UNIQUE,   -- label internal, mis. AST-2611-00012 (dicetak & discan)
    serial_number        VARCHAR(100) NOT NULL,         -- SN pabrikan
    status               VARCHAR(20) NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN (
                             'AVAILABLE','RESERVED','IN_TRANSIT','RENTED',
                             'MAINTENANCE','BROKEN','LOST','RETIRED')),
    condition            VARCHAR(20) NOT NULL DEFAULT 'GOOD'
                             CHECK (condition IN ('GOOD','MINOR_DEFECT','BROKEN')),
    current_warehouse_id BIGINT REFERENCES warehouses(id),
    current_rack_id      BIGINT REFERENCES racks(id),
    current_customer_id  BIGINT,     -- FK di 000005
    current_site_id      BIGINT,     -- FK di 000005
    current_contract_id  BIGINT,     -- FK di 000005
    reserved_for_do_id   BIGINT,     -- FK di 000006
    supplier_id          BIGINT REFERENCES suppliers(id),
    purchase_date        DATE,
    purchase_price       NUMERIC(15, 2),
    warranty_until       DATE,
    notes                TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, serial_number),
    -- Invariant lokasi: unit di gudang wajib punya rak; unit tersewa wajib punya pelanggan
    CONSTRAINT chk_serial_location CHECK (
        (status IN ('AVAILABLE','RESERVED','MAINTENANCE','BROKEN')
            AND current_rack_id IS NOT NULL AND current_customer_id IS NULL)
     OR (status = 'RENTED' AND current_customer_id IS NOT NULL AND current_rack_id IS NULL)
     OR (status IN ('IN_TRANSIT','LOST','RETIRED') AND current_rack_id IS NULL)
    )
);
CREATE INDEX idx_serials_status        ON product_serials(status);
CREATE INDEX idx_serials_rack          ON product_serials(current_rack_id) WHERE current_rack_id IS NOT NULL;
CREATE INDEX idx_serials_customer      ON product_serials(current_customer_id) WHERE current_customer_id IS NOT NULL;
CREATE INDEX idx_serials_sn            ON product_serials(serial_number);

CREATE TABLE lpb_documents (
    id                 BIGSERIAL PRIMARY KEY,
    lpb_number         VARCHAR(50) NOT NULL UNIQUE,
    type               VARCHAR(10) NOT NULL CHECK (type IN ('INBOUND','OUTBOUND')),
    sub_type           VARCHAR(30) NOT NULL CHECK (sub_type IN (
                           -- INBOUND
                           'PURCHASE','INITIAL_STOCK','RENTAL_RETURN','FAILED_DELIVERY_RETURN',
                           'TRANSFER_IN','OPNAME_ADJUSTMENT_IN',
                           -- OUTBOUND
                           'RENTAL_DELIVERY','CONSUMABLE_DELIVERY','SPARE_PART_ISSUE',
                           'TRANSFER_OUT','OPNAME_ADJUSTMENT_OUT','DISPOSAL')),
    status             VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                           'DRAFT','PENDING_APPROVAL','APPROVED','REJECTED','CANCELLED')),
    warehouse_id       BIGINT NOT NULL REFERENCES warehouses(id),
    doc_date           DATE NOT NULL DEFAULT CURRENT_DATE,   -- usecase WAJIB mengisi eksplisit (tanggal WIB via pkg/clock); default ini mengikuti zona DB
    reference_type     VARCHAR(30) CHECK (reference_type IN (
                           'DELIVERY_ORDER','SALES_ORDER','STOCK_OPNAME','MAINTENANCE_TICKET',
                           'TRANSFER','PURCHASE_INVOICE','LPB')),
    reference_id       BIGINT,
    reference_number   VARCHAR(60),
    supplier_id        BIGINT REFERENCES suppliers(id),
    customer_id        BIGINT,       -- FK di 000005
    customer_site_id   BIGINT,       -- FK di 000005
    counterpart_lpb_id BIGINT REFERENCES lpb_documents(id),   -- pasangan TRANSFER_OUT/IN
    notes              TEXT,
    -- Pengiriman (OUTBOUND ke pelanggan / penjemputan unit retur)
    requires_delivery  BOOLEAN NOT NULL DEFAULT FALSE,
    driver_id          BIGINT REFERENCES users(id),
    vehicle_plate      VARCHAR(20),
    delivery_status    VARCHAR(20) CHECK (delivery_status IN (
                           'READY','PICKED_UP','IN_TRANSIT','ARRIVED','DELIVERED','FAILED','RETURNED')),
    delivered_at       TIMESTAMPTZ,
    receiver_name      VARCHAR(100),
    pod_photo_path     TEXT,
    -- Workflow approval
    created_by         BIGINT NOT NULL REFERENCES users(id),
    submitted_by       BIGINT REFERENCES users(id),
    submitted_at       TIMESTAMPTZ,
    approved_by        BIGINT REFERENCES users(id),
    approved_at        TIMESTAMPTZ,
    rejected_by        BIGINT REFERENCES users(id),
    rejected_at        TIMESTAMPTZ,
    rejection_reason   TEXT,
    verify_token       VARCHAR(64) UNIQUE,          -- random, dibuat saat APPROVED (untuk QR publik)
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_lpb_type_subtype CHECK (
        (type = 'INBOUND'  AND sub_type IN ('PURCHASE','INITIAL_STOCK','RENTAL_RETURN',
                                            'FAILED_DELIVERY_RETURN','TRANSFER_IN','OPNAME_ADJUSTMENT_IN'))
     OR (type = 'OUTBOUND' AND sub_type IN ('RENTAL_DELIVERY','CONSUMABLE_DELIVERY','SPARE_PART_ISSUE',
                                            'TRANSFER_OUT','OPNAME_ADJUSTMENT_OUT','DISPOSAL'))
    )
);
CREATE INDEX idx_lpb_status_wh   ON lpb_documents(status, warehouse_id);
CREATE INDEX idx_lpb_driver      ON lpb_documents(driver_id, delivery_status) WHERE driver_id IS NOT NULL;
CREATE INDEX idx_lpb_reference   ON lpb_documents(reference_type, reference_id);

CREATE TABLE lpb_items (
    id          BIGSERIAL PRIMARY KEY,
    lpb_id      BIGINT NOT NULL REFERENCES lpb_documents(id) ON DELETE CASCADE,
    product_id  BIGINT NOT NULL REFERENCES products(id),
    rack_id     BIGINT NOT NULL REFERENCES racks(id),   -- rak asal (OUT) / rak tujuan (IN)
    qty         INT NOT NULL CHECK (qty > 0),
    unit_cost   NUMERIC(15, 2),                          -- utk PURCHASE / INITIAL_STOCK
    do_item_id  BIGINT,                                  -- FK di 000006
    notes       TEXT,
    UNIQUE (lpb_id, product_id, rack_id)
);
CREATE INDEX idx_lpb_items_lpb ON lpb_items(lpb_id);

CREATE TABLE lpb_item_serials (
    id                 BIGSERIAL PRIMARY KEY,
    lpb_item_id        BIGINT NOT NULL REFERENCES lpb_items(id) ON DELETE CASCADE,
    product_serial_id  BIGINT REFERENCES product_serials(id),   -- NULL utk SN baru (PURCHASE) s/d APPROVED
    serial_number      VARCHAR(100) NOT NULL,
    condition_on_scan  VARCHAR(20) NOT NULL DEFAULT 'GOOD'
                           CHECK (condition_on_scan IN ('GOOD','MINOR_DEFECT','BROKEN')),
    scanned_by         BIGINT REFERENCES users(id),
    scanned_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (lpb_item_id, serial_number)
);

-- Buku besar mutasi stok: APPEND-ONLY (dijaga trigger di bawah)
CREATE TABLE stock_ledgers (
    id                 BIGSERIAL PRIMARY KEY,
    occurred_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    product_id         BIGINT NOT NULL REFERENCES products(id),
    warehouse_id       BIGINT NOT NULL REFERENCES warehouses(id),
    rack_id            BIGINT NOT NULL REFERENCES racks(id),
    direction          VARCHAR(3) NOT NULL CHECK (direction IN ('IN','OUT')),
    qty                INT NOT NULL CHECK (qty > 0),
    balance_before     INT NOT NULL,
    balance_after      INT NOT NULL,
    movement_type      VARCHAR(30) NOT NULL,                 -- = lpb_documents.sub_type
    lpb_id             BIGINT NOT NULL REFERENCES lpb_documents(id),
    lpb_item_id        BIGINT NOT NULL REFERENCES lpb_items(id),
    serial_ids         BIGINT[],                             -- id SN yang ikut bergerak (produk SN)
    unit_cost          NUMERIC(15, 2),
    created_by         BIGINT NOT NULL REFERENCES users(id),  -- = approver
    notes              TEXT,
    CONSTRAINT chk_ledger_balance CHECK (
        (direction = 'IN'  AND balance_after = balance_before + qty)
     OR (direction = 'OUT' AND balance_after = balance_before - qty)
    )
);
CREATE INDEX idx_ledgers_product_rack ON stock_ledgers(product_id, rack_id, occurred_at);
CREATE INDEX idx_ledgers_lpb          ON stock_ledgers(lpb_id);
CREATE INDEX idx_ledgers_serials      ON stock_ledgers USING GIN (serial_ids);

-- Riwayat perubahan status/lokasi setiap unit SN (termasuk event non-stok: RESERVED, RENTED, tiket)
CREATE TABLE product_serial_histories (
    id                 BIGSERIAL PRIMARY KEY,
    product_serial_id  BIGINT NOT NULL REFERENCES product_serials(id),
    from_status        VARCHAR(20),
    to_status          VARCHAR(20) NOT NULL,
    from_rack_id       BIGINT REFERENCES racks(id),
    to_rack_id         BIGINT REFERENCES racks(id),
    customer_id        BIGINT,
    event_type         VARCHAR(40) NOT NULL,   -- LPB_APPROVED, DO_RESERVED, DELIVERED, TICKET_OPENED, ...
    reference_type     VARCHAR(30),
    reference_id       BIGINT,
    user_id            BIGINT REFERENCES users(id),
    notes              TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_serial_histories_serial ON product_serial_histories(product_serial_id, created_at);

-- Fungsi forbid_update_delete() sudah didefinisikan di 000001.
CREATE TRIGGER trg_stock_ledgers_append_only
    BEFORE UPDATE OR DELETE ON stock_ledgers
    FOR EACH ROW EXECUTE FUNCTION forbid_update_delete();
CREATE TRIGGER trg_serial_histories_append_only
    BEFORE UPDATE OR DELETE ON product_serial_histories
    FOR EACH ROW EXECUTE FUNCTION forbid_update_delete();

CREATE TRIGGER trg_serials_updated BEFORE UPDATE ON product_serials FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_lpb_updated     BEFORE UPDATE ON lpb_documents   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
