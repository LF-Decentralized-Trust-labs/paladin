ALTER TABLE chained_private_txns ADD COLUMN "contract_address" TEXT;
UPDATE chained_private_txns SET "contract_address" = '';
-- cannot set NOT NULL constraint on SQLite. Not critical to have.