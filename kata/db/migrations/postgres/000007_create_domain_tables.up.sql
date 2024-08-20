BEGIN;

CREATE TABLE onchain_domains (
    "deploy_tx"       UUID    NOT NULL,
    "address"         TEXT    NOT NULL,
    "config_bytes"    TEXT    NOT NULL,
    PRIMARY KEY ("address")
);
-- Index cannot be unique or it's an attack vector to block indexing by deploying a different contract with same deploy TX
CREATE INDEX onchain_domains_deploy_tx ON onchain_domains("deploy_tx");

COMMIT;