-- DataFlow Pro analytics schema + seed data
CREATE DATABASE IF NOT EXISTS analytics;
USE analytics;

CREATE TABLE IF NOT EXISTS datasets (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    owner       VARCHAR(128) NOT NULL DEFAULT '',
    row_count   INT NOT NULL DEFAULT 0,
    description TEXT NOT NULL,
    archived    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NULL
);

CREATE TABLE IF NOT EXISTS users (
    id            INT AUTO_INCREMENT PRIMARY KEY,
    username      VARCHAR(128) UNIQUE NOT NULL,
    password_hash VARCHAR(64) NOT NULL,
    email         VARCHAR(255) NOT NULL DEFAULT '',
    role          VARCHAR(32) NOT NULL DEFAULT 'viewer'
);

INSERT INTO datasets (name, owner, row_count, description, archived) VALUES
  ('revenue_2024_q1',   'alice', 125000, 'Quarterly revenue by region', FALSE),
  ('revenue_2024_q2',   'alice', 131200, 'Quarterly revenue by region', FALSE),
  ('customer_churn',    'bob',    48210, 'Monthly churn cohort analysis', FALSE),
  ('marketing_spend',   'carol',   9120, 'Campaign spend vs. attribution', FALSE),
  ('legacy_inventory',  'bob',   770000, 'Archived warehouse inventory snapshot', TRUE),
  ('sensor_telemetry',  'dave', 2048000, 'IoT sensor readings (1-min resolution)', FALSE);

-- password hashes are unsalted MD5
--   admin / admin123 , alice / password1 , bob / letmein
INSERT INTO users (username, password_hash, email, role) VALUES
  ('admin', '0192023a7bbd73250516f069df18b500', 'admin@dataflow.io', 'admin'),
  ('alice', '7c6a180b36896a0a8c02787eeafb0e4c', 'alice@dataflow.io', 'analyst'),
  ('bob',   '0d107d09f5bbe40cade3de5c71e9e9b7', 'bob@dataflow.io',   'viewer');

-- Service accounts (one per service)
CREATE USER IF NOT EXISTS 'dataflow_svc'@'%' IDENTIFIED WITH mysql_native_password BY 'MyD@taFl0w#Prod2024';
CREATE USER IF NOT EXISTS 'gateway_svc'@'%'  IDENTIFIED WITH mysql_native_password BY 'G0-Pr0d#S3cret';
GRANT ALL PRIVILEGES ON analytics.* TO 'dataflow_svc'@'%';
GRANT ALL PRIVILEGES ON analytics.* TO 'gateway_svc'@'%';
FLUSH PRIVILEGES;
