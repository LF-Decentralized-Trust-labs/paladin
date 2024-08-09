CREATE TABLE event_streams (
    "id"              UUID    NOT NULL,
    "abi"             TEXT    NOT NULL,
    PRIMARY KEY ("id")
);

CREATE TABLE event_stream_checkpoints (
    "stream"          UUID    NOT NULL,
    "block_number"    BIGINT  NOT NULL,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("stream") REFERENCES event_streams ("id") ON DELETE CASCADE
);

CREATE TABLE event_stream_signatures (
    "stream"          UUID    NOT NULL,
    "signature_l"     UUID    NOT NULL,
    "signature_h"     UUID    NOT NULL,
    PRIMARY KEY ("stream", "signature_l", "signature_h"),
    FOREIGN KEY ("stream") REFERENCES event_streams ("id") ON DELETE CASCADE
);

CREATE TABLE event_stream_data (
    "stream"          UUID    NOT NULL,
    "block_number"    BIGINT  NOT NULL,
    "tx_index"        INT     NOT NULL,
    "event_index"     INT     NOT NULL,
    "data"            TEXT,
    PRIMARY KEY ("stream", "block_number", "tx_index", "event_index"),
    FOREIGN KEY ("stream") REFERENCES event_streams ("id") ON DELETE CASCADE,
    FOREIGN KEY ("block_number", "tx_index", "event_index") REFERENCES indexed_events ("block_number", "tx_index", "event_index") ON DELETE CASCADE
);
