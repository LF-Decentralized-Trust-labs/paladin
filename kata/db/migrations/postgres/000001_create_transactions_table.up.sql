BEGIN;
CREATE TABLE transactions (
  seq                       SERIAL          PRIMARY KEY,
  created                   BIGINT          NOT NULL,
  updated                   BIGINT          NOT NULL,

  id                        TEXT            NOT NULL,
  idempotency_key           TEXT            NOT NULL,
  status                    INT             NOT NULL,
  status_message            TEXT,

  -- assembled_pre_req_txs     TEXT,
  -- assembled_payload         TEXT,
  -- assembled_input_states    TEXT,
  -- assembled_output_states   TEXT,
  -- confirmation_tracking_id  TEXT,

  pre_req_txs               TEXT,
  tx_from                      TEXT            NOT NULL,
  tx_contract_address          TEXT            NOT NULL,
  tx_payload                   TEXT            NOT NULL


);

CREATE UNIQUE INDEX transactions_id ON transactions(id);
CREATE UNIQUE INDEX transactions_idempotency_key ON transactions(idempotency_key);
CREATE INDEX transactions_initiator ON transactions(tx_from);
CREATE INDEX transactions_domain_instance ON transactions(tx_contract_address);
-- CREATE INDEX transactions_tracking_id ON transactions(confirmation_tracking_id);
COMMIT;
