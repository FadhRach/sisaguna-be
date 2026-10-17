ALTER TABLE user_vouchers DROP CONSTRAINT IF EXISTS user_vouchers_order_fk;
DROP TABLE IF EXISTS order_items, orders;
