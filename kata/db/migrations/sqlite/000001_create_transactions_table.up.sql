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
);

