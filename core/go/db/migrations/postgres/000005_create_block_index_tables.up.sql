BEGIN;

CREATE TABLE indexed_blocks (
    "hash"            TEXT    NOT NULL,
    "number"          BIGINT  NOT NULL,
    "timestamp"       BIGINT  NOT NULL,
    PRIMARY KEY ("number")
);

CREATE INDEX indexed_blocks_hash ON indexed_blocks("hash");

CREATE TABLE indexed_transactions (
    "hash"              TEXT      NOT NULL,
    "block_number"      BIGINT    NOT NULL,
    "transaction_index" BIGINT    NOT NULL,
    "from"              CHAR(40)  NOT NULL,
    "to"                CHAR(40),
    "nonce"             BIGINT    NOT NULL,
    "contract_address"  CHAR(40),
    "result"            TEXT,
    PRIMARY KEY ("block_number", "transaction_index"),
    FOREIGN KEY ("block_number") REFERENCES indexed_blocks ("number") ON DELETE CASCADE
);
CREATE INDEX indexed_transaction_id ON indexed_transactions("hash");
CREATE UNIQUE INDEX indexed_transaction_from_nonce ON indexed_transactions("from","nonce");

CREATE TABLE indexed_events (
    "transaction_hash"  TEXT    NOT NULL,
    "block_number"      BIGINT  NOT NULL,
    "transaction_index" INT     NOT NULL,
    "log_index"         INT     NOT NULL,
    "signature"         TEXT    NOT NULL,
    PRIMARY KEY ("block_number", "transaction_index", "log_index"),
    FOREIGN KEY ("block_number", "transaction_index") REFERENCES indexed_transactions ("block_number", "transaction_index") ON DELETE CASCADE
);
CREATE INDEX indexed_events_signature ON indexed_events("signature");
CREATE INDEX indexed_events_transaction_hash ON indexed_events("transaction_hash");

COMMIT;