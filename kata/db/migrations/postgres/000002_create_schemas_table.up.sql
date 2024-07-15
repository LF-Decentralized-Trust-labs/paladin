BEGIN;
CREATE TABLE schemas (
    hash_l      UUID,
    hash_h      UUID,
    type        TEXT,
    signature   TEXT,
    content     TEXT,
    labels      TEXT,
    PRIMARY KEY (hash_l, hash_h)
);
COMMIT;