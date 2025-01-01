BEGIN;

-- These tables are replaced (data is not migrated from initial state distribution specific implementation)
DROP TABLE state_distribution_acknowledgments;
DROP TABLE state_distributions;

CREATE TABLE reliable_msgs (
    "sequence"           INTEGER PRIMARY KEY AUTOINCREMENT,
    "id"                 UUID    NOT NULL,
    "created"            BIGINT  NOT NULL,
    "node"               TEXT    NOT NULL,
    "msg_type"           TEXT    NOT NULL,
    "metadata"           TEXT    ,
    PRIMARY KEY ("id")
);

CREATE INDEX reliable_msgs_id ON reliable_msgs ("id");
CREATE INDEX reliable_msgs_node ON reliable_msgs ("node");
CREATE INDEX reliable_msgs_created ON reliable_msgs ("created");

CREATE TABLE reliable_msg_acks (
    "id"                 UUID    NOT NULL,
    "time"               BIGINT  NOT NULL,
    "error"              TEXT,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("id") REFERENCES reliable_msgs ("id") ON DELETE CASCADE
);


