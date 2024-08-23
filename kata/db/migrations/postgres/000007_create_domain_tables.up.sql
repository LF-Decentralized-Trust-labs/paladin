BEGIN;

CREATE TABLE private_smart_contracts (
    "deploy_tx"       UUID    NOT NULL,
    "domain_address"  TEXT    NOT NULL,
    "address"         TEXT    NOT NULL,
    "config_bytes"    TEXT    NOT NULL,
    PRIMARY KEY ("address")
);
CREATE INDEX private_smart_contracts_domain_address ON private_smart_contracts("domain_address");

-- Index cannot be unique or it's an attack vector to block indexing by deploying a different contract with same deploy TX
CREATE INDEX private_smart_contracts_deploy_tx ON private_smart_contracts("deploy_tx");

COMMIT;