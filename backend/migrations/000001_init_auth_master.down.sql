-- 000001_init_auth_master.down.sql
-- Urutan terbalik dari up. Trigger ikut terhapus bersama tabelnya.

DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS racks;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS warehouses;

DROP FUNCTION IF EXISTS forbid_update_delete();
DROP FUNCTION IF EXISTS set_updated_at();
