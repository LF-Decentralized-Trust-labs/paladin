CREATE TABLE schemas (
    "id"             VARCHAR NOT NULL,
    "created_at"     BIGINT,
    "domain_id"      VARCHAR,
    "type"           VARCHAR,
    "signature"      VARCHAR,
    "definition"     VARCHAR,
    "labels"         VARCHAR,
    PRIMARY KEY ("id")
);
