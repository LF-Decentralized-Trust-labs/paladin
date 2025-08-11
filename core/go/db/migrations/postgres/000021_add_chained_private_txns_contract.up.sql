BEGIN;

ALTER TABLE chained_private_txns ADD "contract_address" TEXT;
UPDATE chained_private_txns SET "contract_address" = '';
ALTER TABLE chained_private_txns ALTER COLUMN "contract_address" SET NOT NULL;

COMMIT;
