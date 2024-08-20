CREATE TABLE transactions (
  created_at                TIMESTAMP       NOT NULL,
  updated_at                TIMESTAMP       NOT NULL,
  deleted_at                TIMESTAMP,
  id                        UUID            NOT NULL,
  contract                  VARCHAR         NOT NULL,
  "from"                    VARCHAR         NOT NULL,
  sequence_id               UUID,
  payload_json              VARCHAR,
  payload_rlp               VARCHAR
  
  pre_req_txs               TEXT,
  dispatch_node             TEXT,
  dispatch_address          TEXT,
  dispatch_tx_id            TEXT,
  dispatch_tx_payload       TEXT,
  confirmed_tx_hash         TEXT
);

