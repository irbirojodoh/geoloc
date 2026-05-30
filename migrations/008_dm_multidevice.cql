-- DM multi-device: encrypted identity backup + sender key version on messages
-- Apply with: cqlsh -f migrations/008_dm_multidevice.cql

USE geoloc;

-- Encrypted identity-key backup (passphrase-wrapped on client; opaque to server)
CREATE TABLE IF NOT EXISTS user_dm_identity_backups (
    user_id         UUID,
    backup_version  INT,
    ciphertext      TEXT,
    nonce           TEXT,
    kdf_salt        TEXT,
    updated_at      TIMESTAMP,
    PRIMARY KEY (user_id, backup_version)
) WITH CLUSTERING ORDER BY (backup_version DESC);

-- Which sender public key version was used when the message was encrypted
ALTER TABLE dm_messages ADD sender_key_version INT;
