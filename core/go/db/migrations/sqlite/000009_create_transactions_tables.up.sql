CREATE TABLE abis (
  "hash"                      TEXT            NOT NULL,
  "abi"                       TEXT            NOT NULL,
  "created"                   BIGINT          NOT NULL,
  PRIMARY KEY ("hash")
);
CREATE INDEX abis_created ON abis("created");

CREATE TABLE abi_errors (
  "selector"                  TEXT            NOT NULL,
  "abi_hash"                  TEXT            NOT NULL,
  "full_hash"                 TEXT            NOT NULL,
  "definition"                TEXT            NOT NULL,
  PRIMARY KEY ("abi_hash", "selector"),
  FOREIGN KEY ("abi_hash") REFERENCES abis ("hash") ON DELETE CASCADE
);

CREATE TABLE transactions (
  "id"                        UUID            NOT NULL,
  "idempotency_key"           TEXT,   
  "created"                   BIGINT          NOT NULL,
  "type"                      TEXT            NOT NULL,
  "abi_ref"                   TEXT            NOT NULL,
  "function"                  TEXT,   
  "domain"                    TEXT,   
  "from"                      TEXT            NOT NULL,
  "to"                        TEXT,   
  "data"                      TEXT,   
  PRIMARY KEY ("id"),
  FOREIGN KEY ("abi_ref") REFERENCES abis ("hash") ON DELETE CASCADE
);
CREATE INDEX transactions_created ON transactions("created");
CREATE INDEX transactions_domain ON transactions("domain");
CREATE INDEX transactions_idempotency_key ON transactions("idempotency_key");

CREATE TABLE public_txn_bindings (
  "signer_nonce"              TEXT            NOT NULL,
  "transaction"               UUID            NOT NULL,
  "tx_type"                   TEXT            NOT NULL,
  PRIMARY KEY ("signer_nonce"), -- a binding is not mandatory for a public TXN, but it is singular (see #210)
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
  "failure_message"           TEXT,
  "revert_data"               TEXT,
  "tx_hash"                   TEXT,
  "tx_index"                  INT,
  "log_index"                 INT,
  "source"                    TEXT,
  "block_number"              BIGINT,
  "contract_address"          TEXT,
  PRIMARY KEY ("transaction"),
  FOREIGN KEY ("transaction") REFERENCES transactions ("id") ON DELETE CASCADE
);
CREATE INDEX transaction_receipts_tx_hash ON transaction_receipts("tx_hash");

