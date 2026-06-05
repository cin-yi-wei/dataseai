-- SSH private key authentication for the bastion. When ssh_key_enc is set
-- and non-empty, the tunnel uses key auth (with optional passphrase);
-- otherwise it falls back to ssh_password_enc. Both columns are encrypted
-- with the same cipher as ssh_password_enc.
ALTER TABLE connections ADD COLUMN ssh_key_enc BLOB;
ALTER TABLE connections ADD COLUMN ssh_key_passphrase_enc BLOB;
