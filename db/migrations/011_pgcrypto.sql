-- +goose Up

-- Enable pgcrypto extension for symmetric encryption
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Add encrypted shadow columns for MICR data (dual-write migration strategy)
-- Phase 1: Write to both plaintext and encrypted columns; reads stay on plaintext
-- Phase 2 (future): Switch reads to encrypted columns
-- Phase 3 (future): Drop plaintext columns
ALTER TABLE transfers ADD COLUMN IF NOT EXISTS micr_data_enc BYTEA;
ALTER TABLE transfers ADD COLUMN IF NOT EXISTS routing_number_enc BYTEA;
ALTER TABLE transfers ADD COLUMN IF NOT EXISTS account_number_enc BYTEA;

-- Helper functions for encrypt/decrypt using app.encryption_key session variable
CREATE OR REPLACE FUNCTION encrypt_field(plaintext TEXT) RETURNS BYTEA AS $$
BEGIN
    RETURN pgp_sym_encrypt(plaintext, current_setting('app.encryption_key'));
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION decrypt_field(ciphertext BYTEA) RETURNS TEXT AS $$
BEGIN
    RETURN pgp_sym_decrypt(ciphertext, current_setting('app.encryption_key'));
EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- +goose Down
DROP FUNCTION IF EXISTS decrypt_field(BYTEA);
DROP FUNCTION IF EXISTS encrypt_field(TEXT);
ALTER TABLE transfers DROP COLUMN IF EXISTS account_number_enc;
ALTER TABLE transfers DROP COLUMN IF EXISTS routing_number_enc;
ALTER TABLE transfers DROP COLUMN IF EXISTS micr_data_enc;
DROP EXTENSION IF EXISTS pgcrypto;
