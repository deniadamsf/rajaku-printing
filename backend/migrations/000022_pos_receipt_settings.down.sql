-- 000022_pos_receipt_settings — reverse
DELETE FROM app_settings WHERE key = 'pos.receipt_width_mm';
