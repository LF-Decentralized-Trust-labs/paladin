BEGIN;

CREATE TABLE dispatches (
    "public_transaction_address"  TEXT    NOT NULL,
    "public_transaction_nonce"    BIGINT  NOT NULL,
    "private_transaction_id"      TEXT    NOT NULL,
    "id"                          TEXT    NOT NULL,
    PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX dispatches_public_private ON dispatches("public_transaction_address","public_transaction_nonce","private_transaction_id");

CREATE TABLE state_distributions (
    "state_id"          TEXT    NOT NULL,
    "domain_name"       TEXT    NOT NULL,
    "contract_address"  TEXT    NOT NULL,
    "identity_locator"  TEXT    NOT NULL,
    "id"                TEXT    NOT NULL,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("domain_name", "state_id") REFERENCES states ("domain_name", "id") ON DELETE CASCADE
);

CREATE UNIQUE INDEX state_distributions_state_identity ON state_distributions("state_id","identity_locator");

CREATE TABLE state_distribution_acknowledgments (
    "state_distribution" TEXT    NOT NULL,
    "id"                 TEXT    NOT NULL,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("state_distribution") REFERENCES state_distributions ("id") ON DELETE CASCADE
);

CREATE UNIQUE INDEX state_distribution_acknowledgments_state_distribution ON state_distribution_acknowledgments("state_distribution");

COMMIT;

