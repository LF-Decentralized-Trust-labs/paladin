DROP INDEX transaction_receipts_tx_hash;
DROP INDEX transaction_receipts_source;

ALTER TABLE transaction_receipts RENAME TO  transaction_receipts_old;

-- We need to add the sequence column, but the easiest thing is to simply re-create the table
CREATE TABLE transaction_receipts (
  "sequence"                  INTEGER PRIMARY KEY AUTOINCREMENT,
  "transaction"               UUID            NOT NULL, -- note there is no foreign key to transactions here - we can have receipts for TXs that we do not know locally
  "domain"                    VARCHAR         NOT NULL, -- empty string for public
  "indexed"                   BIGINT          NOT NULL,
  "success"                   BOOLEAN         NOT NULL,
  "failure_message"           VARCHAR,
  "revert_data"               VARCHAR,
  "tx_hash"                   VARCHAR,
  "tx_index"                  INT,
  "log_index"                 INT,
  "source"                    VARCHAR,
  "block_number"              BIGINT,
  "contract_address"          VARCHAR
);
CREATE UNIQUE INDEX transaction_receipts_tx_id ON transaction_receipts ("transaction");
CREATE INDEX transaction_receipts_tx_hash ON transaction_receipts ("tx_hash");
CREATE INDEX transaction_receipts_source ON transaction_receipts ("source");

-- Copy any existing data over
INSERT INTO transaction_receipts (
  "transaction",
  "domain",
  "indexed",
  "success",
  "failure_message",
  "revert_data",
  "tx_hash",
  "tx_index",
  "log_index",
  "source",
  "block_number",
  "contract_address"
) SELECT
  "transaction",
  "domain",
  "indexed",
  "success",
  "failure_message",
  "revert_data",
  "tx_hash",
  "tx_index",
  "log_index",
  "source",
  "block_number",
  "contract_address"
  FROM transaction_receipts_old;

CREATE TABLE receipt_listeners (
    "name"           TEXT       NOT NULL,
    "created"        BIGINT     NOT NULL,
    "started"        BOOLEAN    NOT NULL,
    "filters"        TEXT       NOT NULL,
    "options"        TEXT       NOT NULL,
    PRIMARY KEY("name")
);

CREATE TABLE receipt_listener_gap (
    "listener"           TEXT    NOT NULL,
    "source"             TEXT    NOT NULL,
    "transaction"        UUID    NOT NULL,
    "sequence"           BIGINT  NOT NULL,
    "domain_name"        TEXT    NOT NULL,
    "state"              TEXT    , -- null when a pagination checkpoint
    PRIMARY KEY ("listener", "source"),
    FOREIGN KEY ("listener") REFERENCES receipt_listeners ("name") ON DELETE CASCADE
);

CREATE INDEX receipt_listener_gap_txid ON receipt_listener_gap("transaction");

CREATE TABLE receipt_listener_checkpoints (
    "listener"           TEXT    NOT NULL,
    "sequence"           BIGINT  NOT NULL,
    "time"               BIGINT  NOT NULL,
    PRIMARY KEY ("listener"),
    FOREIGN KEY ("listener") REFERENCES receipt_listeners ("name") ON DELETE CASCADE
);
