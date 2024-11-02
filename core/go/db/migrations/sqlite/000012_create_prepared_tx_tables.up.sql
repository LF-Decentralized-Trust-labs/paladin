CREATE TABLE prepared_txns (
    "id"          UUID       NOT NULL,
    "created"     BIGINT     NOT NULL,
    "domain"      VARCHAR    NOT NULL,
    "to"          VARCHAR    ,
    "transaction" VARCHAR    NOT NULL,
    "metadata"    VARCHAR    ,
    PRIMARY KEY ("id")
    -- FOREIGN KEY ("id") REFERENCES transactions ("id") ON DELETE CASCADE
);

CREATE INDEX prepared_txns_domain_to ON prepared_txns("domain", "to");
CREATE INDEX prepared_txns_created ON  prepared_txns("created");

CREATE TABLE prepared_txn_states (
    "transaction" UUID       NOT NULL,
    "domain_name" VARCHAR,
    "state"       VARCHAR    NOT NULL,
    "state_idx"   INT        NOT NULL,
    "type"        VARCHAR    NOT NULL,
    PRIMARY KEY ("transaction", "type", "state_idx"),
    FOREIGN KEY ("transaction") REFERENCES prepared_txns ("id") ON DELETE CASCADE,
    FOREIGN KEY ("domain_name","state") REFERENCES states ("domain_name","id") ON DELETE CASCADE
);

