CREATE TABLE reg_entries (
    "registry"           TEXT    NOT NULL,
    "id"                 TEXT    NOT NULL,
    "parent_id"          TEXT,
    "name"               TEXT    NOT NULL,
    "created"            BIGINT  NOT NULL,
    "updated"            BIGINT  NOT NULL,
    "active"             BOOLEAN NOT NULL,
    "tx_hash"            TEXT,
    "block_number"       BIGINT,
    "tx_index"           INT,
    "log_index"          INT,
    PRIMARY KEY ("registry", "id"),
    FOREIGN KEY ("registry", "parent_id") REFERENCES reg_entries ("registry", "id") ON DELETE CASCADE
);

-- The name is scoped uniquely within the parent
CREATE UNIQUE INDEX reg_entries_name ON reg_entries("registry", "name", IFNULL("parent_id", ''));


CREATE TABLE reg_props (
    "registry"           TEXT    NOT NULL,
    "entry_id"           TEXT    NOT NULL,
    "name"               TEXT    NOT NULL,
    "created"            BIGINT  NOT NULL,
    "updated"            BIGINT  NOT NULL,
    "active"             BOOLEAN NOT NULL,
    "value"              TEXT    NOT NULL,
    "tx_hash"            TEXT,
    "block_number"       BIGINT,
    "tx_index"           INT,
    "log_index"          INT,
    PRIMARY KEY ("registry", "entry_id", "name"),
    FOREIGN KEY ("registry", "entry_id") REFERENCES reg_entries ("registry", "id") ON DELETE CASCADE
);

-- We deliberately do NOT index the values. The values will be large in most cases.
-- Searches should sub-select based on NOT NULL for existence of properties for efficiency.
-- In the future, if we need the ability to search on some values more efficiently we
-- should introduce a distinction of these fields (using a separate indexed properties)
-- rather than indexing all values of all properties (multi-KB X509 certificates etc.)
