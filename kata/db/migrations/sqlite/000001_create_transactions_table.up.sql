CREATE TABLE transactions (
  created_at                TIMESTAMP       NOT NULL,
  updated_at                TIMESTAMP       NOT NULL,
  deleted_at                TIMESTAMP,
  id                        UUID            NOT NULL,
  contract                  TEXT            NOT NULL,
  "from"                    TEXT            NOT NULL,
  sequence_id               UUID,
  assembled_round           BIGINT,
  payload_json              TEXT,
  payload_rlp               TEXT,
  pre_req_txs               TEXT,
  dispatch_node             TEXT,
  dispatch_address          TEXT,
  dispatch_tx_id            TEXT,
  dispatch_tx_payload       TEXT,
  confirmed_tx_hash         TEXT
);

