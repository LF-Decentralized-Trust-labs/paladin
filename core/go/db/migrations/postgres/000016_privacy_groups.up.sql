BEGIN;

CREATE TABLE privacy_groups (
  "domain"                    TEXT            NOT NULL,
  "id"                        TEXT            NOT NULL,
  "created"                   BIGINT          NOT NULL,
  "genesis_tx"                UUID            NOT NULL,
  "schema_id"                 TEXT            NOT NULL,
  "schema_signature"          TEXT            NOT NULL,
  PRIMARY KEY ( "domain", "id" )
);
CREATE INDEX privacy_groups_created ON privacy_groups ("created");
CREATE INDEX privacy_groups_schema_id ON privacy_groups ("schema_id");

CREATE TABLE privacy_group_members (
    "group"       TEXT    NOT NULL,
    "domain"      TEXT    NOT NULL,
    "idx"         INT     NOT NULL,
    "identity"    TEXT    NOT NULL,
    PRIMARY KEY ("domain", "group", "idx"),
    FOREIGN KEY ("domain", "group") REFERENCES privacy_groups ("domain", "id") ON DELETE CASCADE
);
CREATE INDEX privacy_group_members_identity ON privacy_group_members ("identity");

COMMIT;