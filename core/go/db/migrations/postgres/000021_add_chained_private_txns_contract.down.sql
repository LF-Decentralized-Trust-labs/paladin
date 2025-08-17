BEGIN;

ALTER TABLE chained_private_txns DROP COLUMN "contract_address";

COMMIT;
