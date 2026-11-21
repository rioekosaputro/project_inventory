-- 000004_init_stock_opname.down.sql

ALTER TABLE racks DROP CONSTRAINT IF EXISTS fk_racks_locked_opname;

DROP TABLE IF EXISTS stock_opname_serials;
DROP TABLE IF EXISTS stock_opname_items;
DROP TABLE IF EXISTS stock_opname_racks;
DROP TABLE IF EXISTS stock_opnames;
