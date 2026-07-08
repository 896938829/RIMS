-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: seed role and permission read permissions after route-level RBAC hardening

INSERT INTO permissions (code, name, resource, action)
VALUES
    ('role:list', '查看角色列表', 'role', 'list'),
    ('role:read', '查看角色详情', 'role', 'read'),
    ('permission:list', '查看权限列表', 'permission', 'list')
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
  AND p.code IN ('role:list', 'role:read', 'permission:list')
ON CONFLICT DO NOTHING;
