CREATE TABLE states (
    "id"          VARCHAR NOT NULL,
    "created_at"  BIGINT  NOT NULL,
    "domain_id"   VARCHAR,
    "schema"      VARCHAR,
    "data"        VARCHAR,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("schema") REFERENCES schemas ("id") ON DELETE CASCADE
);

CREATE TABLE state_labels (
    "state"       VARCHAR NOT NULL,
    "label"       VARCHAR NOT NULL,
    "value"       VARCHAR,
    PRIMARY KEY ("state", "label"),
    FOREIGN KEY ("state") REFERENCES states ("id") ON DELETE CASCADE
);
CREATE INDEX state_labels_value ON state_labels("value");

CREATE TABLE state_int64_labels (
    "state"       VARCHAR NOT NULL,
    "label"       VARCHAR NOT NULL,
    "value"       BIGINT,
    PRIMARY KEY ("state", "label"),
    FOREIGN KEY ("state")  REFERENCES states ("id") ON DELETE CASCADE
);
CREATE INDEX state_int64_labels_value ON state_int64_labels("value");
