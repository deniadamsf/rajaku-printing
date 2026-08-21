-- 000020_payment_settings — reverse
DELETE FROM app_settings
WHERE key IN (
    'payment.bank_name',
    'payment.account_name',
    'payment.account_number',
    'payment.qris_note',
    'payment.qris_merchant_name',
    'payment.qris_nmid'
);
