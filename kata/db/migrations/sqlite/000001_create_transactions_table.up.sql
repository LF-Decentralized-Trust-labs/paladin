CREATE TABLE transactions (
  created_at                TIMESTAMP       NOT NULL,
  updated_at                TIMESTAMP       NOT NULL,
  deleted_at                TIMESTAMP,
  id                        UUID            NOT NULL,
  contract                  VARCHAR         NOT NULL,
  "from"                    VARCHAR         NOT NULL,
  sequence_id               UUID,
  domain_id                 VARCHAR,
  schema_id                 VARCHAR,
  payload_json              VARCHAR,
  payload_rlp               VARCHAR,
  assembled_round           BIGINT,
  attestation_plan          VARCHAR,
  pre_req_txs               VARCHAR,
  dispatch_node             VARCHAR,
  dispatch_address          VARCHAR,
  dispatch_tx_id            VARCHAR,
  dispatch_tx_payload       VARCHAR,
  confirmed_tx_hash         VARCHAR
);

