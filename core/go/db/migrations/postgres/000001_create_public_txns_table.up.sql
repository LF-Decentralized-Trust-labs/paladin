BEGIN;

CREATE TABLE public_txns (
  "signer_nonce"              TEXT            NOT NULL,
  "from"                      TEXT            NOT NULL,
  "nonce"                     BIGINT          NOT NULL,
  "created"                   BIGINT          NOT NULL,
  "to"                        TEXT,
  "gas"                       BIGINT          NOT NULL,
  "fixed_gas_pricing"         TEXT,
  "value"                     TEXT,
  "data"                      TEXT,
  "suspended"                 BOOLEAN         NOT NULL,
  PRIMARY KEY("signer_nonce")
);
CREATE UNIQUE INDEX public_txns_from_nonce ON public_txns("from", "nonce");

CREATE TABLE public_submissions (
  "tx_hash"                   TEXT            NOT NULL,
  "signer_nonce"              TEXT            NOT NULL,
  "created"                   BIGINT          NOT NULL,
  "gas_pricing"               TEXT,
  PRIMARY KEY("tx_hash"),
  FOREIGN KEY ("signer_nonce") REFERENCES public_txns ("signer_nonce") ON DELETE CASCADE
);
CREATE INDEX public_submissions_signer_nonce on public_submissions("signer_nonce");

CREATE TABLE public_completions (
  "signer_nonce"              TEXT            NOT NULL,
  "created"                   BIGINT          NOT NULL,
  "tx_hash"                   TEXT            NOT NULL,
  "success"                   BOOLEAN         NOT NULL,
  "revert_data"               TEXT,
  FOREIGN KEY ("signer_nonce") REFERENCES public_txns ("signer_nonce") ON DELETE CASCADE,
  PRIMARY KEY("signer_nonce")
);

COMMIT;
