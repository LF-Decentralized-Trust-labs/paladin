ALTER TABLE public_txn_bindings ADD "contract_address" TEXT;
UPDATE public_txn_bindings SET "contract_address" = '';
