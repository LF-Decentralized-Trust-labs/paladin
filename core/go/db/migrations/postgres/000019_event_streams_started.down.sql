BEGIN;

ALTER TABLE "event_streams" DROP COLUMN "started";

COMMIT;