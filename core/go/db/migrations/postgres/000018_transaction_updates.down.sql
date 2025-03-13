BEGIN;

DROP TABLE IF EXISTS transaction_history;

DROP INDEX "public_submissions_pub_txn_id";
CREATE UNIQUE INDEX public_submissions_pub_txn_id on public_submissions("pub_txn_id");

COMMIT;