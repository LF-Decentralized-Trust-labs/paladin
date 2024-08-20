BEGIN;

CREATE TABLE state_confirms (
    "state"       TEXT    NOT NULL,
    "transaction" UUID    NOT NULL,
    PRIMARY KEY ("state")
);
CREATE INDEX state_confirm_transaction ON state_confirms("transaction");

CREATE TABLE state_spends (
    "state"       TEXT    NOT NULL,
    "transaction" UUID    NOT NULL,
    PRIMARY KEY ("state")
);
CREATE INDEX state_spend_transaction ON state_spends("transaction");

CREATE TABLE state_locks (
    "state"       TEXT    NOT NULL,
    "sequence"    UUID    NOT NULL,
    "creating"    BOOLEAN NOT NULL,
    "spending"    BOOLEAN NOT NULL,
    PRIMARY KEY ("state")
);
CREATE INDEX state_lock_sequence ON state_locks("sequence");


COMMIT;