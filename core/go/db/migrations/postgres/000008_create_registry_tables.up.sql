BEGIN;

CREATE TABLE registry_transport_details (
    "node"               TEXT    NOT NULL,
    "registry"           TEXT    NOT NULL,
    "transport"          TEXT    NOT NULL,
    "transport_details"  TEXT    NOT NULL,
    PRIMARY KEY ("registry","node","transport")
);

CREATE UNIQUE INDEX node_transport ON registry("node");

COMMIT;