-- T1.8: tujuan transfer disimpan per item. Hanya terisi pada LPB TRANSFER_OUT (rak tujuan; boleh di gudang lain).
-- Pasangan TRANSFER_IN dibuat saat TRANSFER_OUT disetujui (lpb_documents.counterpart_lpb_id sudah ada sejak 000003).
ALTER TABLE lpb_items ADD COLUMN dest_rack_id BIGINT REFERENCES racks(id);
ALTER TABLE lpb_items ADD CONSTRAINT chk_lpb_item_dest_ne_src CHECK (dest_rack_id IS NULL OR dest_rack_id <> rack_id);
CREATE INDEX idx_lpb_documents_counterpart ON lpb_documents(counterpart_lpb_id) WHERE counterpart_lpb_id IS NOT NULL;
