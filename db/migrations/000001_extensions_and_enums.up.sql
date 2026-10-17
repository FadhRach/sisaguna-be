CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TYPE listing_tier      AS ENUM ('human', 'animal_feed', 'compost');
CREATE TYPE quantity_unit     AS ENUM ('portion', 'kg');
CREATE TYPE storage_method    AS ENUM ('room_temp', 'shelf', 'chilled', 'frozen');
CREATE TYPE safety_check      AS ENUM ('untouched', 'sensory_ok', 'stored_right', 'packed_clean', 'hygiene');
CREATE TYPE halal_status      AS ENUM ('halal_certified', 'non_halal', 'unverified', 'other');
CREATE TYPE allergen          AS ENUM ('seafood', 'peanut', 'tree_nut', 'milk', 'egg', 'gluten', 'soy', 'sesame');
CREATE TYPE merchant_status   AS ENUM ('pending', 'approved', 'rejected', 'suspended');
CREATE TYPE listing_status    AS ENUM ('active', 'paused', 'deleted');
CREATE TYPE order_status      AS ENUM ('pending_payment', 'ready', 'completed', 'cancelled', 'no_show', 'expired');
CREATE TYPE fulfillment       AS ENUM ('pickup', 'delivery');
CREATE TYPE payment_kind      AS ENUM ('cash', 'qris', 'gopay', 'ovo', 'dana');
CREATE TYPE payment_status    AS ENUM ('unpaid', 'pending', 'paid', 'failed', 'expired', 'refunded');
CREATE TYPE courier           AS ENUM ('gosend', 'grab', 'shopee', 'lalamove');
CREATE TYPE delivery_speed    AS ENUM ('priority', 'standard', 'saver');
CREATE TYPE complaint_reason  AS ENUM ('not_fresh', 'wrong_item', 'missing', 'packaging', 'hygiene', 'merchant', 'other');
CREATE TYPE complaint_status  AS ENUM ('submitted', 'reviewing', 'resolved', 'rejected');
CREATE TYPE notification_type AS ENUM ('pickup', 'order', 'merchant', 'promo', 'impact');
CREATE TYPE chat_side         AS ENUM ('buyer', 'merchant');
CREATE TYPE media_kind        AS ENUM ('image', 'video');
CREATE TYPE media_visibility  AS ENUM ('public', 'private');

CREATE FUNCTION set_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END $$;
