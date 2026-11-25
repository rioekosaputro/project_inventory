-- 000006_init_customers_contracts.down.sql

ALTER TABLE lpb_documents
    DROP CONSTRAINT IF EXISTS fk_lpb_site,
    DROP CONSTRAINT IF EXISTS fk_lpb_customer;

ALTER TABLE product_serials
    DROP CONSTRAINT IF EXISTS fk_serial_contract,
    DROP CONSTRAINT IF EXISTS fk_serial_site,
    DROP CONSTRAINT IF EXISTS fk_serial_customer;

DROP TABLE IF EXISTS rental_placements;
DROP TABLE IF EXISTS rental_contract_items;
DROP TABLE IF EXISTS rental_contracts;
DROP TABLE IF EXISTS customer_sites;
DROP TABLE IF EXISTS customers;
