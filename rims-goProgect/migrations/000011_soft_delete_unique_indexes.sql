-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: convert soft-delete-sensitive unique keys to partial indexes

ALTER TABLE roles DROP CONSTRAINT IF EXISTS roles_code_key;
ALTER TABLE permissions DROP CONSTRAINT IF EXISTS permissions_code_key;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_username_key;

DROP INDEX IF EXISTS idx_roles_code;
DROP INDEX IF EXISTS idx_permissions_code;
DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS uni_roles_code;
DROP INDEX IF EXISTS uni_permissions_code;
DROP INDEX IF EXISTS uni_users_username;

CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_code_active ON roles(code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_permissions_code_active ON permissions(code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_active ON users(username) WHERE deleted_at IS NULL;
