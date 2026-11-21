-- 000003_init_inventory_lpb.down.sql
-- Urutan terbalik dari up. Trigger append-only ikut terhapus bersama tabelnya
-- (DROP TABLE tidak memicu trigger baris).

DROP TABLE IF EXISTS product_serial_histories;
DROP TABLE IF EXISTS stock_ledgers;
DROP TABLE IF EXISTS lpb_item_serials;
DROP TABLE IF EXISTS lpb_items;
DROP TABLE IF EXISTS lpb_documents;
DROP TABLE IF EXISTS product_serials;
DROP TABLE IF EXISTS stock_balances;
DROP TABLE IF EXISTS document_sequences;
