CREATE TABLE receipt_listener_incomplete (
    "listener"           TEXT    NOT NULL,
    "transaction"        UUID    NOT NULL,
    "sequence"           BIGINT  NOT NULL,
    "domain_name"        TEXT    NOT NULL,
    "created"            BIGINT  NOT NULL,
    PRIMARY KEY ("listener", "transaction"),
    FOREIGN KEY ("listener") REFERENCES receipt_listeners ("name") ON DELETE CASCADE
);

CREATE INDEX receipt_listener_incomplete_domain ON receipt_listener_incomplete("listener", "domain_name");
CREATE INDEX receipt_listener_incomplete_sequence ON receipt_listener_incomplete("listener", "sequence");
