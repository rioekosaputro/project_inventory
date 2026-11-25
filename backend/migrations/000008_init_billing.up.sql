-- 000008_init_billing.up.sql (Fase 2 — T2.1)
-- Pengaturan tarif PPN (PMK 131/2024), invoice, item invoice, dan pembayaran + PPh 23.

CREATE TABLE tax_settings (
    id               BIGSERIAL PRIMARY KEY,
    tax_code         VARCHAR(20) NOT NULL DEFAULT 'PPN',
    rate             NUMERIC(5, 2) NOT NULL,          -- 12.00
    dpp_numerator    INT NOT NULL DEFAULT 11,         -- DPP Nilai Lain = harga x 11/12
    dpp_denominator  INT NOT NULL DEFAULT 12,
    faktur_code      VARCHAR(5) NOT NULL DEFAULT '04',
    effective_from   DATE NOT NULL,
    effective_to     DATE,
    notes            TEXT,
    UNIQUE (tax_code, effective_from)
);

CREATE TABLE invoices (
    id                    BIGSERIAL PRIMARY KEY,
    invoice_number        VARCHAR(50) NOT NULL UNIQUE,
    invoice_type          VARCHAR(20) NOT NULL CHECK (invoice_type IN ('RENTAL','CONSUMABLE','SERVICE','OTHER')),
    customer_id           BIGINT NOT NULL REFERENCES customers(id),
    contract_id           BIGINT REFERENCES rental_contracts(id),
    so_id                 BIGINT REFERENCES sales_orders(id),
    billing_period_start  DATE,
    billing_period_end    DATE,
    invoice_date          DATE NOT NULL,
    due_date              DATE NOT NULL,
    status                VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN (
                              'DRAFT','ISSUED','PARTIALLY_PAID','PAID','VOID')),
    is_ppn                BOOLEAN NOT NULL,
    subtotal              NUMERIC(15, 2) NOT NULL DEFAULT 0,
    discount_amount       NUMERIC(15, 2) NOT NULL DEFAULT 0,
    dpp                   NUMERIC(15, 2) NOT NULL DEFAULT 0,
    dpp_other_value       NUMERIC(15, 2) NOT NULL DEFAULT 0,
    tax_rate              NUMERIC(5, 2)  NOT NULL DEFAULT 0,
    tax_amount            NUMERIC(15, 2) NOT NULL DEFAULT 0,
    grand_total           NUMERIC(15, 2) NOT NULL DEFAULT 0,
    paid_amount           NUMERIC(15, 2) NOT NULL DEFAULT 0,
    faktur_pajak_number   VARCHAR(40),
    pdf_path              TEXT,
    notes                 TEXT,
    generated_by_job      BOOLEAN NOT NULL DEFAULT FALSE,
    created_by            BIGINT REFERENCES users(id),
    issued_by             BIGINT REFERENCES users(id),
    issued_at             TIMESTAMPTZ,
    voided_by             BIGINT REFERENCES users(id),
    voided_at             TIMESTAMPTZ,
    void_reason           TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_invoice_period CHECK (
        invoice_type <> 'RENTAL'
        OR (contract_id IS NOT NULL AND billing_period_start IS NOT NULL
            AND billing_period_end >= billing_period_start))
);
CREATE UNIQUE INDEX uq_invoice_rental_period
    ON invoices(contract_id, billing_period_start, billing_period_end)
    WHERE invoice_type = 'RENTAL' AND status <> 'VOID';
CREATE INDEX idx_invoices_customer ON invoices(customer_id, status);
CREATE INDEX idx_invoices_due      ON invoices(due_date) WHERE status IN ('ISSUED','PARTIALLY_PAID');

CREATE TABLE invoice_items (
    id                  BIGSERIAL PRIMARY KEY,
    invoice_id          BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description         TEXT NOT NULL,
    product_id          BIGINT REFERENCES products(id),
    product_serial_id   BIGINT REFERENCES product_serials(id),
    customer_site_id    BIGINT REFERENCES customer_sites(id),
    rental_placement_id BIGINT REFERENCES rental_placements(id),
    period_start        DATE,
    period_end          DATE,
    days_billed         INT,
    days_in_period      INT,
    qty                 NUMERIC(12, 4) NOT NULL DEFAULT 1,
    unit_price          NUMERIC(15, 2) NOT NULL,
    line_total          NUMERIC(15, 2) NOT NULL
);
CREATE INDEX idx_invoice_items_invoice ON invoice_items(invoice_id);

CREATE TABLE payments (
    id                 BIGSERIAL PRIMARY KEY,
    invoice_id         BIGINT NOT NULL REFERENCES invoices(id),
    payment_date       DATE NOT NULL,
    amount_received    NUMERIC(15, 2) NOT NULL CHECK (amount_received >= 0),
    pph23_amount       NUMERIC(15, 2) NOT NULL DEFAULT 0 CHECK (pph23_amount >= 0),
    pph23_slip_number  VARCHAR(60),
    method             VARCHAR(20) NOT NULL CHECK (method IN ('TRANSFER','CASH','GIRO','OTHER')),
    reference_number   VARCHAR(80),
    proof_path         TEXT,
    notes              TEXT,
    recorded_by        BIGINT NOT NULL REFERENCES users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_payment_positive CHECK (amount_received + pph23_amount > 0)
);
CREATE INDEX idx_payments_invoice ON payments(invoice_id);

CREATE TRIGGER trg_invoices_updated BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION set_updated_at();
