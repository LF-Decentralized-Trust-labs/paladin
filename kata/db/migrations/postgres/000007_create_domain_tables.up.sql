BEGIN;

CREATE TABLE onchain_domains (
    "address"         TEXT    NOT NULL,
    "config_bytes"    TEXT    NOT NULL,
    PRIMARY KEY ("address")
);

COMMIT;