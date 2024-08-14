BEGIN;

CREATE TABLE states (
    "id"          TEXT    NOT NULL,
    "created_at"  BIGINT  NOT NULL,
    "domain_id"   TEXT,
    "schema"      TEXT,
    "data"        TEXT,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("schema") REFERENCES schemas ("id") ON DELETE CASCADE
);

CREATE TABLE state_labels (
    "state"       TEXT    NOT NULL,
    "label"       TEXT    NOT NULL,
    "value"       TEXT,
    PRIMARY KEY ("state", "label"),
    FOREIGN KEY ("state") REFERENCES states ("id") ON DELETE CASCADE
);
CREATE INDEX state_labels_value ON state_labels("value");

CREATE TABLE state_int64_labels (
    "state"       TEXT    NOT NULL,
    "label"       TEXT    NOT NULL,
    "value"       BIGINT,
    PRIMARY KEY ("state", "label"),
    FOREIGN KEY ("state")  REFERENCES states ("id") ON DELETE CASCADE
);
CREATE INDEX state_int64_labels_value ON state_int64_labels("value");

COMMIT;