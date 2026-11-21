-- 000001_init_auth_master.up.sql
-- Fungsi umum, user & auth, master gudang/rak/kategori/produk/supplier

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Dipakai trigger append-only (audit_logs di 000002, stock_ledgers & product_serial_histories di 000003)
CREATE OR REPLACE FUNCTION forbid_update_delete() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Tabel % bersifat append-only: % tidak diizinkan', TG_TABLE_NAME, TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE warehouses (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(20) UNIQUE NOT NULL,          -- WH-SMG-01
    name        VARCHAR(100) NOT NULL,
    address     TEXT,
    phone       VARCHAR(30),
    latitude    DECIMAL(10, 7),
    longitude   DECIMAL(10, 7),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(100) NOT NULL,
    email          VARCHAR(120) NOT NULL UNIQUE,
    phone          VARCHAR(30),
    password_hash  TEXT NOT NULL,
    role           VARCHAR(30) NOT NULL CHECK (role IN (
                       'ADMIN','MANAGEMENT','BUSINESS_ADMIN',
                       'STAFF_WAREHOUSE','HEAD_WAREHOUSE','DRIVER','TECHNICIAN')),
    warehouse_id   BIGINT REFERENCES warehouses(id),  -- scope gudang utk role warehouse
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    must_change_password BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE refresh_tokens (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   CHAR(64) NOT NULL UNIQUE,            -- sha256 hex, token asli tidak disimpan
    family_id    UUID NOT NULL,                       -- deteksi reuse token curian
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    replaced_by  BIGINT REFERENCES refresh_tokens(id),
    user_agent   TEXT,
    ip_address   INET,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);

CREATE TABLE racks (
    id            BIGSERIAL PRIMARY KEY,
    warehouse_id  BIGINT NOT NULL REFERENCES warehouses(id),
    code          VARCHAR(30) NOT NULL,               -- A-02-01 (Zona-Rak-Level)
    zone          VARCHAR(30),                        -- PRINTER, SPAREPART, KARANTINA, WORKSHOP
    is_sellable   BOOLEAN NOT NULL DEFAULT TRUE,      -- FALSE utk KARANTINA/WORKSHOP: tidak dihitung available
    description   TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    locked_by_opname_id BIGINT,                       -- FK ditambah di 000005
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (warehouse_id, code)
);

CREATE TABLE categories (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL UNIQUE,          -- Printer, PC, Monitor, Toner, Sparepart
    item_type  VARCHAR(20) NOT NULL CHECK (item_type IN ('ASSET', 'CONSUMABLE')),
    is_active  BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE products (
    id                 BIGSERIAL PRIMARY KEY,
    sku                VARCHAR(50) UNIQUE NOT NULL,   -- juga dipakai sbg barcode barang non-SN
    name               VARCHAR(150) NOT NULL,
    category_id        BIGINT NOT NULL REFERENCES categories(id),
    brand              VARCHAR(80),
    model              VARCHAR(80),
    unit               VARCHAR(20) NOT NULL DEFAULT 'unit',
    has_serial_number  BOOLEAN NOT NULL DEFAULT FALSE,
    min_stock_alert    INT NOT NULL DEFAULT 0 CHECK (min_stock_alert >= 0),
    default_monthly_rate NUMERIC(15, 2),              -- tarif sewa acuan (bisa dioverride di kontrak)
    default_sell_price NUMERIC(15, 2),                -- harga jual consumable
    description        TEXT,
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_products_category ON products(category_id);

CREATE TABLE suppliers (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(30) UNIQUE NOT NULL,
    name        VARCHAR(150) NOT NULL,
    pic_name    VARCHAR(100),
    phone       VARCHAR(30),
    email       VARCHAR(120),
    npwp        VARCHAR(30),
    address     TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_warehouses_updated BEFORE UPDATE ON warehouses FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_users_updated      BEFORE UPDATE ON users      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_racks_updated      BEFORE UPDATE ON racks      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_products_updated   BEFORE UPDATE ON products   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_suppliers_updated  BEFORE UPDATE ON suppliers  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
