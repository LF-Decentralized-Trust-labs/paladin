BEGIN;

CREATE TABLE abis (
  "hash"                      VARCHAR         NOT NULL,
  "abi"                       VARCHAR         NOT NULL,
  "created"                   BIGINT          NOT NULL,
  PRIMARY KEY ("hash")
);
CREATE INDEX abis_created ON abis("created");

CREATE TABLE abi_errors (
  "selector"                  VARCHAR         NOT NULL,
  "full_hash"                 VARCHAR         NOT NULL,
  "abi_hash"                  VARCHAR         NOT NULL,
  "definition"                VARCHAR         NOT NULL,
  PRIMARY KEY ("abi_hash", "selector"),
  FOREIGN KEY ("abi_hash") REFERENCES abis ("hash") ON DELETE CASCADE
);

CREATE TABLE transactions (
  "id"                        UUID            NOT NULL,
  "idempotency_key"           VARCHAR,
  "created"                   BIGINT          NOT NULL,
  "type"                      VARCHAR         NOT NULL,
  "abi_ref"                   VARCHAR         NOT NULL,
  "function"                  VARCHAR,
  "domain"                    VARCHAR,
  "from"                      VARCHAR         NOT NULL,
  "to"                        VARCHAR,
  "data"                      VARCHAR,
  PRIMARY KEY ("id"),
  FOREIGN KEY ("abi_ref") REFERENCES abis ("hash") ON DELETE CASCADE
);
CREATE INDEX transactions_created ON transactions("created");
CREATE INDEX transactions_domain ON transactions("domain");
CREATE INDEX transactions_idempotency_key ON transactions("idempotency_key");

CREATE TABLE public_txn_bindings (
  "sequence"                  BIGSERIAL       PRIMARY KEY, -- allows us to use insertion order to order lists
  "signer_nonce"              VARCHAR         NOT NULL,
  "transaction"               UUID            NOT NULL,
  "tx_type"                   VARCHAR         NOT NULL,
  FOREIGN KEY ("transaction") REFERENCES transactions ("id") ON DELETE CASCADE,
  FOREIGN KEY ("signer_nonce") REFERENCES public_txns ("signer_nonce") ON DELETE CASCADE
);
CREATE INDEX public_txn_bindings_transaction ON public_txn_bindings("transaction");
CREATE INDEX public_txn_bindings_signer_nonce ON public_txn_bindings("signer_nonce");

CREATE TABLE transaction_deps (
  "transaction"               UUID            NOT NULL,
  "depends_on"                UUID            NOT NULL,
  PRIMARY KEY ("transaction","depends_on"),
  FOREIGN KEY ("transaction") REFERENCES transactions ("id") ON DELETE CASCADE
);
CREATE INDEX transaction_deps_depends_on ON transaction_deps("depends_on");

CREATE TABLE transaction_receipts (
  "transaction"               UUID            NOT NULL,
  "indexed"                   BIGINT          NOT NULL,
  "success"                   BOOLEAN         NOT NULL,
  "failure_message"           VARCHAR,
  "revert_data"               VARCHAR,
  "tx_hash"                   VARCHAR,
  "block_number"              BIGINT,
  PRIMARY KEY ("transaction"),
  FOREIGN KEY ("transaction") REFERENCES transactions ("id") ON DELETE CASCADE
);
CREATE INDEX transaction_receipts_tx_hash ON transaction_receipts ("tx_hash");

COMMIT;
