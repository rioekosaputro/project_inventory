DROP INDEX IF EXISTS idx_lpb_documents_counterpart;
ALTER TABLE lpb_items DROP CONSTRAINT IF EXISTS chk_lpb_item_dest_ne_src;
ALTER TABLE lpb_items DROP COLUMN IF EXISTS dest_rack_id;
