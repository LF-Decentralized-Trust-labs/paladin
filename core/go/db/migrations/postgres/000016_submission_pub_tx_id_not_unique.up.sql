BEGIN;

-- Allow multiple submissions for the same public transaction
DROP INDEX "public_submissions_pub_txn_id";
CREATE INDEX public_submissions_pub_txn_id on public_submissions("pub_txn_id");

COMMIT;