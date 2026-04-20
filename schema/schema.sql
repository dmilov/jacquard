CREATE DATABASE IF NOT EXISTS jacquard;
USE jacquard;

CREATE TABLE IF NOT EXISTS conversations (
    id         VARCHAR(36)  NOT NULL PRIMARY KEY,
    node_id    VARCHAR(255) NOT NULL,
    command    TEXT         NOT NULL,
    started_at DATETIME     NOT NULL,
    ended_at   DATETIME     NULL,
    INDEX idx_node_started (node_id, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS messages (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    conversation_id VARCHAR(36)             NOT NULL,
    role            ENUM('user','assistant') NOT NULL,
    content         TEXT                    NOT NULL,
    sequence        INT                     NOT NULL,
    created_at      DATETIME                NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
    INDEX idx_conv_seq (conversation_id, sequence)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
