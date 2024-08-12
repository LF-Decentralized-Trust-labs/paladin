CREATE TABLE indexed_blocks (
    "hash_l"          UUID    NOT NULL,
    "hash_h"          UUID    NOT NULL,
    "number"          BIGINT  NOT NULL,
    PRIMARY KEY ("number")
);

CREATE TABLE indexed_transactions (
    "hash_l"            UUID      NOT NULL,
    "hash_h"            UUID      NOT NULL,
    "block_number"      BIGINT    NOT NULL,
    "transaction_index" BIGINT    NOT NULL,
    "from"              CHAR(40)  NOT NULL,
    "to"                CHAR(40),
    "contract_address"  CHAR(40),
    PRIMARY KEY ("block_number", "transaction_index"),
    FOREIGN KEY ("block_number") REFERENCES indexed_blocks ("number") ON DELETE CASCADE
);
CREATE INDEX indexed_transaction_id ON indexed_transactions("hash_l", "hash_h");

CREATE TABLE indexed_events (
    "transaction_l"     UUID    NOT NULL,
    "transaction_h"     UUID    NOT NULL,
    "block_number"      BIGINT  NOT NULL,
    "transaction_index" INT     NOT NULL,
    "log_index"         INT     NOT NULL,
    "signature_l"       UUID    NOT NULL,
    "signature_h"       UUID    NOT NULL,
    PRIMARY KEY ("block_number", "transaction_index", "log_index"),
    FOREIGN KEY ("block_number", "transaction_index") REFERENCES indexed_transactions ("block_number", "transaction_index") ON DELETE CASCADE
);
CREATE INDEX indexed_events_signature ON indexed_events("signature_l", "signature_h");
