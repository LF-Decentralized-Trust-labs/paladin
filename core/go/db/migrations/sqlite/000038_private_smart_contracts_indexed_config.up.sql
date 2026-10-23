ALTER TABLE private_smart_contracts ADD COLUMN "indexed_config_bytes" TEXT NOT NULL DEFAULT '';
UPDATE private_smart_contracts SET "indexed_config_bytes" = "config_bytes";

-- Existing rows do not get backfilled with an event location. This migration is part of the same
-- release that adds config upgrades which means NULL can simply be handled as older than any event
-- location string.
ALTER TABLE private_smart_contracts ADD COLUMN "config_location" TEXT;
