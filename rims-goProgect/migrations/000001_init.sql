-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: users, roles, permissions

-- Roles
CREATE TABLE IF NOT EXISTS roles (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(32) NOT NULL UNIQUE,
    name VARCHAR(64) NOT NULL,
    description VARCHAR(255) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_roles_deleted_at ON roles(deleted_at);

-- Permissions
CREATE TABLE IF NOT EXISTS permissions (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(64) NOT NULL,
    resource VARCHAR(32) NOT NULL,
    action VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_permissions_resource ON permissions(resource);
CREATE INDEX IF NOT EXISTS idx_permissions_deleted_at ON permissions(deleted_at);

-- Role-Permission mapping
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- Users
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    real_name VARCHAR(64) DEFAULT '',
    phone VARCHAR(20) DEFAULT '',
    email VARCHAR(128) DEFAULT '',
    role_id BIGINT NOT NULL REFERENCES roles(id),
    status SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

-- Seed default roles
INSERT INTO roles (code, name, description) VALUES
    ('admin', '管理员', '系统管理员，拥有所有权限'),
    ('user', '普通用户', '普通操作用户')
ON CONFLICT (code) DO NOTHING;

-- Seed default admin user (password: admin123, bcrypt hash)
INSERT INTO users (username, password_hash, real_name, role_id, status) VALUES
    ('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', '系统管理员',
     (SELECT id FROM roles WHERE code = 'admin'), 1)
ON CONFLICT (username) DO NOTHING;
