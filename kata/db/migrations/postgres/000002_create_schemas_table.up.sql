BEGIN;
CREATE TABLE schemas (
    "id"             TEXT    NOT NULL,
    "created_at"     BIGINT,
    "domain_id"      TEXT,
    "type"           TEXT,
    "signature"      TEXT,
    "definition"     TEXT,
    "labels"         TEXT,
    PRIMARY KEY ("id")
);
COMMIT;