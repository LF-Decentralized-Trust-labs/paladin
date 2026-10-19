BEGIN;

ALTER TABLE private_smart_contracts ADD COLUMN "indexed_config_bytes" TEXT;
UPDATE private_smart_contracts SET "indexed_config_bytes" = "config_bytes";
ALTER TABLE private_smart_contracts ALTER COLUMN "indexed_config_bytes" SET NOT NULL;

-- Existing rows do not get backfilled with an event location. This migration is part of the same
-- release that adds config upgrades which means NULL can simply be handled as older than any event
-- location string.
ALTER TABLE private_smart_contracts ADD COLUMN "config_location" TEXT;

COMMIT;
