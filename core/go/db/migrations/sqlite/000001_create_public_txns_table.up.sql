CREATE TABLE public_txns (
  "signer_nonce"              VARCHAR         NOT NULL,
  "from"                      VARCHAR         NOT NULL,
  "nonce"                     BIGINT          NOT NULL,
  "created"                   BIGINT          NOT NULL,
  "key_handle"                VARCHAR         NOT NULL,
  "to"                        VARCHAR,
  "gas"                       BIGINT          NOT NULL,
  "fixed_gas_pricing"         VARCHAR,
  "value"                     VARCHAR,
  "data"                      VARCHAR,
  "suspended"                 BOOLEAN         NOT NULL,
  PRIMARY KEY("signer_nonce")
);

CREATE UNIQUE INDEX public_txns_from_nonce ON public_txns("from", "nonce");

CREATE TABLE public_submissions (
  "signer_nonce"              VARCHAR         NOT NULL,
  "created"                   BIGINT          NOT NULL,
  "tx_hash"                   VARCHAR         NOT NULL,
  "gas_pricing"               VARCHAR,
  FOREIGN KEY ("signer_nonce") REFERENCES public_txns ("signer_nonce") ON DELETE CASCADE,
  PRIMARY KEY("signer_nonce")
);

CREATE TABLE public_completions (
  "signer_nonce"              VARCHAR         NOT NULL,
  "created"                   BIGINT          NOT NULL,
  "tx_hash"                   VARCHAR         NOT NULL,
  "success"                   BOOLEAN         NOT NULL,
  "revert_data"               VARCHAR,
  FOREIGN KEY ("signer_nonce") REFERENCES public_txns ("signer_nonce") ON DELETE CASCADE,
  PRIMARY KEY("signer_nonce")
);