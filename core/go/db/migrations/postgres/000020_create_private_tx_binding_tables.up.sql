BEGIN;

CREATE TABLE chained_private_txns (
  "chained_transaction"       UUID            NOT NULL REFERENCES transactions ("id") ON DELETE CASCADE,
  "transaction"               UUID            NOT NULL, -- we might not be the originator of the original transaction, so might not have it in our DB
  "sender"                    TEXT            NOT NULL, -- sender of the original transaction, so we know where to route the receipt
  "domain"                    TEXT            NOT NULL,
  PRIMARY KEY ("chained_transaction", "transaction")
);

COMMIT;
