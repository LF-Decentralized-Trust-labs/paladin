BEGIN;

CREATE TABLE event_streams (
    "id"              UUID    NOT NULL,
    "created"         BIGINT  NOT NULL,
    "updated"         BIGINT  NOT NULL,
    "type"            TEXT    NOT NULL,
    "name"            TEXT    NOT NULL,
    "config"          TEXT    NOT NULL,
    "abi"             TEXT    NOT NULL,
    PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX event_stream_name ON event_streams("type","name");

CREATE TABLE event_stream_checkpoints (
    "stream"          UUID    NOT NULL,
    "block_number"    BIGINT  NOT NULL,
    PRIMARY KEY ("stream"),
    FOREIGN KEY ("stream") REFERENCES event_streams ("id") ON DELETE CASCADE
);

COMMIT;