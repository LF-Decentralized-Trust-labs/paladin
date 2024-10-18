BEGIN;

CREATE TABLE key_paths (
    "parent"             VARCHAR         NOT NULL,
    "index"              BIGINT          NOT NULL,
    "path"               VARCHAR         NOT NULL,
    PRIMARY KEY ("parent", "index")
);

CREATE UNIQUE INDEX key_paths_path ON key_paths ("path");

CREATE TABLE key_mappings (
    "identifier"         VARCHAR         NOT NULL,
    "wallet"             VARCHAR         NOT NULL,
    "key_handle"         VARCHAR         NOT NULL,
    PRIMARY KEY ("identifier"),
    FOREIGN KEY ("identifier") REFERENCES key_paths ("path") ON DELETE CASCADE
);

CREATE UNIQUE INDEX key_mappings_identifier ON key_mappings ("identifier");

CREATE TABLE key_verifiers (
    "identifier"         VARCHAR         NOT NULL,
    "algorithm"          VARCHAR         NOT NULL,
    "type"               VARCHAR         NOT NULL,
    "verifier"           VARCHAR         NOT NULL,
    PRIMARY KEY ("verifier", "algorithm", "type"), -- globally unique across wallets
    FOREIGN KEY ("identifier") REFERENCES key_mappings ("identifier") ON DELETE CASCADE
);

CREATE UNIQUE INDEX key_verifiers_identifier ON key_verifiers ("identifier", "algorithm", "type");

COMMIT;
