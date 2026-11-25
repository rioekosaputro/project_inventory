-- 000006_init_customers_contracts.up.sql (Fase 2 — T2.1)
-- Pelanggan, cabang/site, kontrak sewa, item tarif kontrak, penempatan unit (placement).

CREATE TABLE customers (
    id                   BIGSERIAL PRIMARY KEY,
    customer_code        VARCHAR(30) UNIQUE NOT NULL,
    company_name         VARCHAR(150) NOT NULL,
    pic_name             VARCHAR(100) NOT NULL,
    phone                VARCHAR(30) NOT NULL,
    email                VARCHAR(120),
    is_pkp               BOOLEAN NOT NULL DEFAULT TRUE,
    npwp                 VARCHAR(30),            -- NPWP 16 digit (format Coretax)
    tax_address          TEXT,                   -- alamat sesuai NPWP utk faktur
    billing_address      TEXT NOT NULL,
    billing_postal_code  VARCHAR(10) NOT NULL,
    billing_email        VARCHAR(120),           -- tujuan kirim invoice
    payment_term_days    INT NOT NULL DEFAULT 30 CHECK (payment_term_days >= 0),
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_by           BIGINT REFERENCES users(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- KOREKSI: ON DELETE RESTRICT + soft delete (bukan CASCADE)
CREATE TABLE customer_sites (
    id            BIGSERIAL PRIMARY KEY,
    customer_id   BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    site_name     VARCHAR(100) NOT NULL,
    pic_name      VARCHAR(100) NOT NULL,
    pic_phone     VARCHAR(30) NOT NULL,
    address_line  TEXT NOT NULL,
    subdistrict   VARCHAR(80),
    district      VARCHAR(80),
    city          VARCHAR(80) NOT NULL,
    province      VARCHAR(80) NOT NULL,
    postal_code   VARCHAR(10) NOT NULL,
    latitude      DECIMAL(10, 7),
    longitude     DECIMAL(10, 7),
    is_primary    BOOLEAN NOT NULL DEFAULT FALSE,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_customer_sites_customer ON customer_sites(customer_id);
CREATE UNIQUE INDEX uq_customer_primary_site ON customer_sites(customer_id) WHERE is_primary AND is_active;

CREATE TABLE rental_contracts (
    id                    BIGSERIAL PRIMARY KEY,
    contract_number       VARCHAR(50) UNIQUE NOT NULL,
    customer_id           BIGINT NOT NULL REFERENCES customers(id),
    start_date            DATE NOT NULL,
    end_date              DATE NOT NULL,
    duration_months       INT NOT NULL CHECK (duration_months > 0),
    billing_day_of_month  SMALLINT NOT NULL DEFAULT 1 CHECK (billing_day_of_month BETWEEN 1 AND 28),
    billing_mode          VARCHAR(20) NOT NULL DEFAULT 'PRORATA'
                              CHECK (billing_mode IN ('PRORATA','FULL_MONTH')),
    billing_in_advance    BOOLEAN NOT NULL DEFAULT FALSE, -- MVP: tagih di belakang (arrears)
    is_ppn                BOOLEAN NOT NULL DEFAULT TRUE,
    payment_term_days     INT NOT NULL DEFAULT 30,
    deposit_amount        NUMERIC(15, 2) NOT NULL DEFAULT 0,
    status                VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
                              CHECK (status IN ('DRAFT','ACTIVE','EXPIRED','TERMINATED')),
    terminated_at         DATE,
    termination_reason    TEXT,
    renewed_from_id       BIGINT REFERENCES rental_contracts(id),
    signed_document_path  TEXT,
    notes                 TEXT,
    created_by            BIGINT NOT NULL REFERENCES users(id),
    activated_by          BIGINT REFERENCES users(id),
    activated_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_contract_dates CHECK (end_date > start_date)
);
CREATE INDEX idx_contracts_customer ON rental_contracts(customer_id, status);

CREATE TABLE rental_contract_items (
    id                BIGSERIAL PRIMARY KEY,
    contract_id       BIGINT NOT NULL REFERENCES rental_contracts(id) ON DELETE CASCADE,
    product_id        BIGINT NOT NULL REFERENCES products(id),
    customer_site_id  BIGINT REFERENCES customer_sites(id),
    qty               INT NOT NULL CHECK (qty > 0),
    monthly_rate      NUMERIC(15, 2) NOT NULL CHECK (monthly_rate >= 0),
    notes             TEXT,
    UNIQUE (contract_id, product_id, customer_site_id)
);

-- Periode penempatan unit di pelanggan = dasar hitung tagihan prorata.
-- Interval [start_date, end_date): hari start ditagih, hari end (keluar) tidak ditagih.
CREATE TABLE rental_placements (
    id                 BIGSERIAL PRIMARY KEY,
    contract_id        BIGINT NOT NULL REFERENCES rental_contracts(id),
    contract_item_id   BIGINT REFERENCES rental_contract_items(id),
    product_serial_id  BIGINT NOT NULL REFERENCES product_serials(id),
    customer_site_id   BIGINT NOT NULL REFERENCES customer_sites(id),
    monthly_rate       NUMERIC(15, 2) NOT NULL,
    start_date         DATE NOT NULL,
    end_date           DATE,
    start_lpb_id       BIGINT REFERENCES lpb_documents(id),
    end_lpb_id         BIGINT REFERENCES lpb_documents(id),
    end_reason         VARCHAR(20) CHECK (end_reason IN ('WITHDRAWAL','SWAP','CONTRACT_END','LOST')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_placement_dates CHECK (end_date IS NULL OR end_date >= start_date)
);
CREATE UNIQUE INDEX uq_active_placement ON rental_placements(product_serial_id) WHERE end_date IS NULL;
CREATE INDEX idx_placements_contract ON rental_placements(contract_id, start_date, end_date);

ALTER TABLE product_serials
    ADD CONSTRAINT fk_serial_customer FOREIGN KEY (current_customer_id) REFERENCES customers(id),
    ADD CONSTRAINT fk_serial_site     FOREIGN KEY (current_site_id)     REFERENCES customer_sites(id),
    ADD CONSTRAINT fk_serial_contract FOREIGN KEY (current_contract_id) REFERENCES rental_contracts(id);

ALTER TABLE lpb_documents
    ADD CONSTRAINT fk_lpb_customer FOREIGN KEY (customer_id)      REFERENCES customers(id),
    ADD CONSTRAINT fk_lpb_site     FOREIGN KEY (customer_site_id) REFERENCES customer_sites(id);

CREATE TRIGGER trg_customers_updated BEFORE UPDATE ON customers        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_sites_updated     BEFORE UPDATE ON customer_sites   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_contracts_updated BEFORE UPDATE ON rental_contracts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
