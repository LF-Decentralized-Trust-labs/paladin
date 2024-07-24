BEGIN;

CREATE TABLE states (
    hash_l      UUID    NOT NULL,
    hash_h      UUID    NOT NULL,
    created_at  BIGINT  NOT NULL,
    domain_id   TEXT,
    schema_l    UUID,
    schema_h    UUID,
    data        TEXT,
    PRIMARY KEY (hash_l, hash_h),
    FOREIGN KEY (schema_l, schema_h) REFERENCES schemas (hash_l, hash_h) ON DELETE CASCADE
);

CREATE TABLE state_labels (
    state_l     UUID    NOT NULL,
    state_h     UUID    NOT NULL,   
    label       TEXT    NOT NULL,
    value       TEXT,
    PRIMARY KEY (state_l, state_h, label),
    FOREIGN KEY (state_l, state_h) REFERENCES states (hash_l, hash_h) ON DELETE CASCADE
);
CREATE INDEX state_labels_value ON state_labels(value);

CREATE TABLE state_int64_labels (
    state_l     UUID    NOT NULL,
    state_h     UUID    NOT NULL,   
    label       TEXT    NOT NULL,
    value       BIGINT,
    PRIMARY KEY (state_l, state_h, label),
    FOREIGN KEY (state_l, state_h) REFERENCES states (hash_l, hash_h) ON DELETE CASCADE
);
CREATE INDEX state_int64_labels_value ON state_int64_labels(value);

COMMIT;