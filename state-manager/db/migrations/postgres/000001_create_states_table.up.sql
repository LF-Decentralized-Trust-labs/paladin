BEGIN;
CREATE TABLE states (
  seq             SERIAL,
  id              VARCHAR(64)     NOT NULL PRIMARY KEY,
  created         BIGINT          NOT NULL,
  updated         BIGINT          NOT NULL,
  state           VARCHAR(64)     NOT NULL
);

CREATE UNIQUE INDEX states_id ON states(id);
CREATE INDEX states_state ON states(state);
COMMIT;
