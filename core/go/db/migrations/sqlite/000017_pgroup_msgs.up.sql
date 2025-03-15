CREATE TABLE pgroup_msgs (
  "local_seq"                 INTEGER         PRIMARY KEY AUTOINCREMENT,
  "domain"                    TEXT            NOT NULL,
  "group"                     TEXT            NOT NULL,
  "node"                      TEXT            NOT NULL,
  "sent"                      BIGINT          NOT NULL,
  "received"                  BIGINT          NOT NULL,
  "id"                        UUID            NOT NULL,
  "cid"                       UUID            ,
  "topic"                     TEXT            NOT NULL,
  "data"                      TEXT            NOT NULL,
  FOREIGN KEY ("domain", "group") REFERENCES privacy_groups ("domain", "id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX pgroup_msgs_id ON pgroup_msgs ("id");
CREATE INDEX pgroup_msgs_cid ON pgroup_msgs ("cid");
CREATE INDEX pgroup_msgs_group ON pgroup_msgs("domain","group");

CREATE TABLE message_listeners (
    "name"           TEXT       NOT NULL,
    "created"        BIGINT     NOT NULL,
    "started"        BOOLEAN    NOT NULL,
    "filters"        TEXT       NOT NULL,
    "options"        TEXT       NOT NULL,
    PRIMARY KEY("name")
);

CREATE TABLE message_listener_checkpoints (
    "listener"           TEXT    NOT NULL,
    "sequence"           BIGINT  NOT NULL,
    "time"               BIGINT  NOT NULL,
    PRIMARY KEY ("listener"),
    FOREIGN KEY ("listener") REFERENCES message_listeners ("name") ON DELETE CASCADE
);
