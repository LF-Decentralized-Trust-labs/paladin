CREATE TABLE schemas (
    hash_l        UUID,
    hash_h        UUID,
    created_at    BIGINT,
    domain_id     VARCHAR,
    type          VARCHAR,
    signature     VARCHAR,
    definition    VARCHAR,
    labels        VARCHAR,
    PRIMARY KEY (hash_l, hash_h)
);
