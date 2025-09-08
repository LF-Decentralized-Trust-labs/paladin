BEGIN;

ALTER TABLE public_txn_bindings DROP COLUMN "sender";

COMMIT;
