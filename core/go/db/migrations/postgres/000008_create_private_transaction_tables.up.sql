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

CREATE TABLE transaction_delegations (
  "id"                        UUID NOT NULL,
  "transaction_id"            UUID NOT NULL,
  "delegate_node_id"          TEXT NOT NULL,
  PRIMARY KEY ("id")
  -- need to reorder the migrations before we can define a foreign key to the transactions table  FOREIGN KEY ("transaction_id") REFERENCES transactions ("id") ON DELETE CASCADE
);

CREATE TABLE transaction_delegation_acknowledgements (
  "delegation"            UUID NOT NULL,
  "id"                    UUID NOT NULL,
  PRIMARY KEY ("id"),
  FOREIGN KEY ("delegation") REFERENCES transaction_delegations ("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX transaction_delegation_acknowledgements_delegation ON transaction_delegation_acknowledgements("delegation");

COMMIT;

