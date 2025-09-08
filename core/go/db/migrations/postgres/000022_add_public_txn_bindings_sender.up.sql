BEGIN;

ALTER TABLE public_txn_bindings ADD "sender" TEXT;
UPDATE public_txn_bindings SET "sender" = '';
ALTER TABLE public_txn_bindings ALTER COLUMN "sender" SET NOT NULL;

COMMIT;
