-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: warehouses, user_warehouses

-- Warehouses
CREATE TABLE IF NOT EXISTS warehouses (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(32) NOT NULL UNIQUE,
    name VARCHAR(128) NOT NULL,
    status SMALLINT NOT NULL DEFAULT 1,
    address VARCHAR(255) DEFAULT '',
    contact_person VARCHAR(64) DEFAULT '',
    contact_phone VARCHAR(20) DEFAULT '',
    created_by BIGINT NOT NULL DEFAULT 0,
    updated_by BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_warehouses_deleted_at ON warehouses(deleted_at);
CREATE INDEX IF NOT EXISTS idx_warehouses_status ON warehouses(status);

-- User-Warehouse bindings
CREATE TABLE IF NOT EXISTS user_warehouses (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE CASCADE,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_warehouse
    ON user_warehouses(user_id, warehouse_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_warehouses_warehouse_id ON user_warehouses(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_user_warehouses_deleted_at ON user_warehouses(deleted_at);

-- Seed default warehouse
INSERT INTO warehouses (code, name, status, created_by, updated_by) VALUES
    ('WH001', '默认仓库', 1, 0, 0)
ON CONFLICT (code) DO NOTHING;

-- Bind admin user to default warehouse
INSERT INTO user_warehouses (user_id, warehouse_id, is_default)
SELECT u.id, w.id, TRUE
FROM users u, warehouses w
WHERE u.username = 'admin' AND w.code = 'WH001'
AND NOT EXISTS (
    SELECT 1 FROM user_warehouses uw
    WHERE uw.user_id = u.id AND uw.warehouse_id = w.id AND uw.deleted_at IS NULL
);
