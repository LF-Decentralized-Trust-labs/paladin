BEGIN;

-- Allow multiple submissions for the same public transaction
DROP INDEX "public_submissions_pub_txn_id";
CREATE INDEX public_submissions_pub_txn_id on public_submissions("pub_txn_id");

-- Create a new table to store a record of previous transaction values
CREATE TABLE transaction_history (
  "id"                        UUID            NOT NULL,
  "tx_id"                     UUID            NOT NULL,
  "idempotency_key"           TEXT,
  "created"                   BIGINT          NOT NULL,
  "type"                      TEXT            NOT NULL,
  "abi_ref"                   TEXT            NOT NULL,
  "function"                  TEXT,
  "domain"                    TEXT,
  "from"                      TEXT            NOT NULL,
  "to"                        TEXT,
  "data"                      TEXT,
  "gas"                       BIGINT,
  "value"                     TEXT,
  "gas_price"                 TEXT,
  "max_fee_per_gas"           TEXT,
  "max_priority_fee_per_gas"  TEXT,

  PRIMARY KEY ("id"),
  FOREIGN KEY ("abi_ref") REFERENCES abis ("hash") ON DELETE CASCADE,
  FOREIGN KEY ("tx_id") REFERENCES transactions ("id") ON DELETE CASCADE
);

COMMIT;