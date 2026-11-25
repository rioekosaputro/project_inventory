-- 000007_init_sales_delivery.up.sql (Fase 2 — T2.1)
-- Sales Orders (4 tipe), Delivery Orders, dan reservasi stok gudang.

CREATE TABLE sales_orders (
    id                BIGSERIAL PRIMARY KEY,
    so_number         VARCHAR(50) NOT NULL UNIQUE,
    order_type        VARCHAR(20) NOT NULL CHECK (order_type IN (
                          'NEW_DELIVERY','WITHDRAWAL','UNIT_SWAP','CONSUMABLE')),
    customer_id       BIGINT NOT NULL REFERENCES customers(id),
    customer_site_id  BIGINT NOT NULL REFERENCES customer_sites(id),
    contract_id       BIGINT REFERENCES rental_contracts(id),
    status            VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                          'DRAFT','CONFIRMED','IN_PROGRESS','COMPLETED','CANCELLED')),
    order_date        DATE NOT NULL DEFAULT CURRENT_DATE,
    requested_date    DATE,
    is_ppn            BOOLEAN NOT NULL DEFAULT TRUE,
    subtotal          NUMERIC(15, 2) NOT NULL DEFAULT 0,
    discount_amount   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    dpp               NUMERIC(15, 2) NOT NULL DEFAULT 0,
    dpp_other_value   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    tax_rate          NUMERIC(5, 2)  NOT NULL DEFAULT 0,
    tax_amount        NUMERIC(15, 2) NOT NULL DEFAULT 0,
    grand_total       NUMERIC(15, 2) NOT NULL DEFAULT 0,
    related_ticket_id BIGINT,                                  -- FK di migrasi Fase 3
    notes             TEXT,
    created_by        BIGINT NOT NULL REFERENCES users(id),
    confirmed_by      BIGINT REFERENCES users(id),
    confirmed_at      TIMESTAMPTZ,
    cancelled_by      BIGINT REFERENCES users(id),
    cancelled_at      TIMESTAMPTZ,
    cancel_reason     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_so_contract CHECK (order_type = 'CONSUMABLE' OR contract_id IS NOT NULL)
);
CREATE INDEX idx_so_customer ON sales_orders(customer_id, status);

CREATE TABLE sales_order_items (
    id                 BIGSERIAL PRIMARY KEY,
    so_id              BIGINT NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    product_id         BIGINT NOT NULL REFERENCES products(id),
    direction          VARCHAR(3) NOT NULL DEFAULT 'OUT' CHECK (direction IN ('OUT','IN')),
    qty                INT NOT NULL CHECK (qty > 0),
    unit_price         NUMERIC(15, 2) NOT NULL DEFAULT 0,
    line_total         NUMERIC(15, 2) NOT NULL DEFAULT 0,
    product_serial_id  BIGINT REFERENCES product_serials(id),
    notes              TEXT
);
CREATE INDEX idx_so_items_so ON sales_order_items(so_id);

CREATE TABLE delivery_orders (
    id              BIGSERIAL PRIMARY KEY,
    do_number       VARCHAR(50) NOT NULL UNIQUE,
    so_id           BIGINT NOT NULL REFERENCES sales_orders(id),
    warehouse_id    BIGINT NOT NULL REFERENCES warehouses(id),
    direction       VARCHAR(10) NOT NULL CHECK (direction IN ('OUTBOUND','INBOUND')),
    status          VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN (
                        'OPEN','RESERVED','LPB_CREATED','COMPLETED','CANCELLED')),
    scheduled_date  DATE,
    lpb_id          BIGINT REFERENCES lpb_documents(id),
    notes           TEXT,
    created_by      BIGINT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_do_so ON delivery_orders(so_id);
CREATE INDEX idx_do_wh_status ON delivery_orders(warehouse_id, status);

CREATE TABLE delivery_order_items (
    id           BIGSERIAL PRIMARY KEY,
    do_id        BIGINT NOT NULL REFERENCES delivery_orders(id) ON DELETE CASCADE,
    so_item_id   BIGINT NOT NULL REFERENCES sales_order_items(id),
    product_id   BIGINT NOT NULL REFERENCES products(id),
    qty          INT NOT NULL CHECK (qty > 0)
);

CREATE TABLE delivery_order_reservations (
    id                 BIGSERIAL PRIMARY KEY,
    do_item_id         BIGINT NOT NULL REFERENCES delivery_order_items(id) ON DELETE CASCADE,
    rack_id            BIGINT NOT NULL REFERENCES racks(id),
    product_serial_id  BIGINT REFERENCES product_serials(id),
    qty                INT NOT NULL CHECK (qty > 0),
    reserved_by        BIGINT NOT NULL REFERENCES users(id),
    reserved_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at        TIMESTAMPTZ,
    CONSTRAINT chk_reservation_serial_qty CHECK (product_serial_id IS NULL OR qty = 1)
);
CREATE UNIQUE INDEX uq_active_serial_reservation
    ON delivery_order_reservations(product_serial_id)
    WHERE product_serial_id IS NOT NULL AND released_at IS NULL;

ALTER TABLE lpb_items
    ADD CONSTRAINT fk_lpb_items_do_item FOREIGN KEY (do_item_id) REFERENCES delivery_order_items(id);
ALTER TABLE product_serials
    ADD CONSTRAINT fk_serial_reserved_do FOREIGN KEY (reserved_for_do_id) REFERENCES delivery_orders(id);

CREATE TRIGGER trg_so_updated BEFORE UPDATE ON sales_orders    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_do_updated BEFORE UPDATE ON delivery_orders FOR EACH ROW EXECUTE FUNCTION set_updated_at();
