CREATE TABLE registry (
    "node"               TEXT    NOT NULL,
    "registry"           TEXT    NOT NULL,
    "transport"          TEXT    NOT NULL,
    "transport_details"  TEXT    NOT NULL,
    PRIMARY KEY ("node","registry","transport")
);

CREATE UNIQUE INDEX node_transport ON registry("node");
