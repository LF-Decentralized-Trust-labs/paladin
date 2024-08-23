CREATE TABLE state_confirms (
    "state"       VARCHAR NOT NULL,
    "transaction" UUID    NOT NULL,
    PRIMARY KEY ("state")
);
CREATE INDEX state_confirm_transaction ON state_confirms("transaction");

CREATE TABLE state_spends (
    "state"       VARCHAR NOT NULL,
    "transaction" UUID    NOT NULL,
    PRIMARY KEY ("state")
);
CREATE INDEX state_spend_transaction ON state_spends("transaction");

CREATE TABLE state_locks (
    "state"       VARCHAR NOT NULL,
    "transaction" UUID    NOT NULL,
    "creating"    BOOLEAN NOT NULL,
    "spending"    BOOLEAN NOT NULL,
    PRIMARY KEY ("state")
);
CREATE INDEX state_lock_transaction ON state_locks("transaction");
