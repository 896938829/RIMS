-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: seed read permission codes added after route-level RBAC hardening

INSERT INTO permissions (code, name, resource, action)
VALUES
    ('user:list', '查看用户列表', 'user', 'list'),
    ('user:read', '查看用户详情', 'user', 'read'),
    ('warehouse:read', '查看仓库', 'warehouse', 'read')
ON CONFLICT (code) WHERE deleted_at IS NULL DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    deleted_at = NULL,
    updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles AS r
CROSS JOIN permissions AS p
WHERE r.code = 'admin'
  AND r.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND p.code IN ('user:list', 'user:read', 'warehouse:read')
ON CONFLICT DO NOTHING;
