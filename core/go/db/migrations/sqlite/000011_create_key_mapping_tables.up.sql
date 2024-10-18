
CREATE TABLE key_paths (
    "parent"             TEXT            NOT NULL,
    "index"              BIGINT          NOT NULL,
    "path"               TEXT            NOT NULL,
    PRIMARY KEY ("parent", "index")
);

CREATE UNIQUE INDEX key_paths_path ON key_paths ("path");

CREATE TABLE key_mappings (
    "identifier"         TEXT            NOT NULL,
    "wallet"             TEXT            NOT NULL,
    "key_handle"         TEXT            NOT NULL,
    PRIMARY KEY ("identifier"),
    FOREIGN KEY ("identifier") REFERENCES key_paths ("path") ON DELETE CASCADE
);

CREATE UNIQUE INDEX key_mappings_identifier ON key_mappings ("identifier");

CREATE TABLE key_verifiers (
    "identifier"         TEXT            NOT NULL,
    "algorithm"          TEXT            NOT NULL,
    "type"               TEXT            NOT NULL,
    "verifier"           TEXT            NOT NULL,
    PRIMARY KEY ("verifier", "algorithm", "type"), -- globally unique across wallets
    FOREIGN KEY ("identifier") REFERENCES key_mappings ("identifier") ON DELETE CASCADE
);

CREATE UNIQUE INDEX key_verifiers_identifier ON key_verifiers ("identifier", "algorithm", "type");
