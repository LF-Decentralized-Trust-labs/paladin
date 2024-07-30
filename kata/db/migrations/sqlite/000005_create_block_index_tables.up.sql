CREATE TABLE indexed_blocks (
    "hash_l"          UUID    NOT NULL,
    "hash_h"          UUID    NOT NULL,
    "number"          BIGINT  NOT NULL,
    PRIMARY KEY ("number")
);

CREATE TABLE indexed_transactions (
    "hash_l"          UUID    NOT NULL,
    "hash_h"          UUID    NOT NULL,
    "block_number"    BIGINT  NOT NULL,
    "tx_index"        BIGINT  NOT NULL,
    PRIMARY KEY ("hash_l", "hash_h"),
    FOREIGN KEY ("block_number") REFERENCES indexed_blocks ("number") ON DELETE CASCADE
);
CREATE INDEX indexed_transaction_sort ON indexed_transactions("block_number", "tx_index");

CREATE TABLE indexed_events (
    "transaction_l"   UUID    NOT NULL,
    "transaction_h"   UUID    NOT NULL,
    "block_number"    BIGINT  NOT NULL,
    "tx_index"        INT     NOT NULL,
    "event_index"     INT     NOT NULL,
    "signature_l"     UUID    NOT NULL,
    "signature_h"     UUID    NOT NULL,
    PRIMARY KEY ("block_number", "tx_index", "event_index"),
    FOREIGN KEY ("block_number", "tx_index") REFERENCES indexed_transactions ("block_number", "tx_index") ON DELETE CASCADE
);
CREATE INDEX indexed_events_signature ON indexed_events("signature_l", "signature_h");


