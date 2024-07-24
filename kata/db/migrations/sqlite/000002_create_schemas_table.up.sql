CREATE TABLE schemas (
    hash_l        UUID     NOT NULL,
    hash_h        UUID     NOT NULL,
    created_at    BIGINT,
    domain_id     VARCHAR,
    type          VARCHAR,
    signature     VARCHAR,
    definition    VARCHAR,
    labels        VARCHAR,
    PRIMARY KEY (hash_l, hash_h)
);
