-- SSH tunnel support: route the MySQL TCP connection through an SSH bastion.
ALTER TABLE connections ADD COLUMN ssh_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE connections ADD COLUMN ssh_host TEXT NOT NULL DEFAULT '';
ALTER TABLE connections ADD COLUMN ssh_port INTEGER NOT NULL DEFAULT 22;
ALTER TABLE connections ADD COLUMN ssh_user TEXT NOT NULL DEFAULT '';
ALTER TABLE connections ADD COLUMN ssh_password_enc BLOB;
