BEGIN;

-- These tables are replaced (data is not migrated from initial state distribution specific implementation)
DROP TABLE state_distribution_acknowledgments;
DROP TABLE state_distributions;

CREATE TABLE queued_msgs (
    "id"                 TEXT    NOT NULL,
    "created"            BIGINT  NOT NULL,
    "cid"                TEXT    ,
    "node"               TEXT    NOT NULL,
    "component"          TEXT    NOT NULL,
    "reply_to"           TEXT    NOT NULL,
    "type"               TEXT    NOT NULL,
    "payload"            TEXT    ,
    PRIMARY KEY ("id")
);

CREATE INDEX queued_msgs_node ON queued_msgs ("node");

CREATE TABLE queued_msg_acks (
    "id"                 TEXT    NOT NULL,
    "acked"              BIGINT  NOT NULL,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("id") REFERENCES queued_msgs ("id") ON DELETE CASCADE
);


