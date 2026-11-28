-- 000009_init_tracking_tickets.down.sql

DROP TRIGGER IF EXISTS trg_tickets_updated ON maintenance_tickets;

ALTER TABLE sales_orders DROP CONSTRAINT IF EXISTS fk_so_ticket;

DROP TABLE IF EXISTS ticket_activities;
DROP TABLE IF EXISTS ticket_spareparts;
DROP TABLE IF EXISTS maintenance_tickets;
DROP TABLE IF EXISTS shipment_tracking_logs;
