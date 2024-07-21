BEGIN;

CREATE TABLE state_confirms (
    "state_l"     UUID    NOT NULL,
    "state_h"     UUID    NOT NULL,
    "transaction" UUID    NOT NULL,
    PRIMARY KEY (state_l, state_h)
);
CREATE INDEX state_confirm_transaction ON state_confirms("transaction");

CREATE TABLE state_spends (
    "state_l"     UUID    NOT NULL,
    "state_h"     UUID    NOT NULL,
    "transaction" UUID    NOT NULL,
    PRIMARY KEY (state_l, state_h)
);
CREATE INDEX state_spend_transaction ON state_spends("transaction");

CREATE TABLE state_locks (
    "state_l"     UUID    NOT NULL,
    "state_h"     UUID    NOT NULL,
    "sequence"    UUID    NOT NULL,
    PRIMARY KEY (state_l, state_h)
);
CREATE INDEX state_lock_sequence ON state_locks("sequence");


COMMIT;