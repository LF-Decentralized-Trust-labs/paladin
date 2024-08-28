BEGIN;
CREATE TABLE transactions (
  created_at                TIMESTAMP       NOT NULL,
  updated_at                TIMESTAMP       NOT NULL,
  deleted_at                TIMESTAMP       ,
  id                        UUID            NOT NULL default gen_random_uuid(),
  contract                  TEXT            NOT NULL,
  "from"                    TEXT            NOT NULL,
  sequence_id               UUID,
  domain_id                 TEXT,
  schema_id                 TEXT,
  assembled_round           BIGINT,
  attestation_plan          TEXT,
  attestation_results       TEXT,
  payload_json              TEXT,
  payload_rlp               TEXT,
  pre_req_txs               TEXT,
  dispatch_node             TEXT,
  dispatch_address          TEXT,
  dispatch_tx_id            TEXT,
  dispatch_tx_payload       TEXT,
  confirmed_tx_hash         TEXT

);

CREATE UNIQUE INDEX transactions_id ON transactions(id);
COMMIT;
