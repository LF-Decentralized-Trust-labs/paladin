BEGIN;

ALTER TABLE private_smart_contracts DROP COLUMN "config_location";
ALTER TABLE private_smart_contracts DROP COLUMN "indexed_config_bytes";

COMMIT;
