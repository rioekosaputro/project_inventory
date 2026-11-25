-- 000007_init_sales_delivery.down.sql

ALTER TABLE product_serials
    DROP CONSTRAINT IF EXISTS fk_serial_reserved_do;
ALTER TABLE lpb_items
    DROP CONSTRAINT IF EXISTS fk_lpb_items_do_item;

DROP TABLE IF EXISTS delivery_order_reservations;
DROP TABLE IF EXISTS delivery_order_items;
DROP TABLE IF EXISTS delivery_orders;
DROP TABLE IF EXISTS sales_order_items;
DROP TABLE IF EXISTS sales_orders;
