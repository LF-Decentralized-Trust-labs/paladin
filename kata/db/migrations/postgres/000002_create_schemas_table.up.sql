BEGIN;
CREATE TABLE schemas (
    hash_l         UUID    NOT NULL,
    hash_h         UUID    NOT NULL,
    created_at     BIGINT,
    domain_id      TEXT,
    type           TEXT,
    signature      TEXT,
    definition     TEXT,
    labels         TEXT,
    PRIMARY KEY (hash_l, hash_h)
);
COMMIT;