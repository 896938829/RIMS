-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: seed route-level RBAC permission codes

INSERT INTO permissions (code, name, resource, action)
VALUES
    ('user:create', '创建用户', 'user', 'create'),
    ('user:update', '更新用户', 'user', 'update'),
    ('user:delete', '删除用户', 'user', 'delete'),
    ('user:reset_password', '重置用户密码', 'user', 'reset_password'),
    ('role:create', '创建角色', 'role', 'create'),
    ('role:update', '更新角色', 'role', 'update'),
    ('role:delete', '删除角色', 'role', 'delete'),
    ('role:assign_permissions', '分配角色权限', 'role', 'assign_permissions'),
    ('warehouse:create', '创建仓库', 'warehouse', 'create'),
    ('warehouse:update', '更新仓库', 'warehouse', 'update'),
    ('warehouse:delete', '删除仓库', 'warehouse', 'delete'),
    ('warehouse:bind_user', '绑定仓库用户', 'warehouse', 'bind_user'),
    ('warehouse:unbind_user', '解绑仓库用户', 'warehouse', 'unbind_user'),
    ('warehouse:list_users', '查看仓库用户', 'warehouse', 'list_users'),
    ('product:create', '创建商品', 'product', 'create'),
    ('product:update', '更新商品', 'product', 'update'),
    ('product:delete', '删除商品', 'product', 'delete'),
    ('inventory:update', '更新库存设置', 'inventory', 'update'),
    ('non_std:create', '创建非标库存', 'non_std', 'create'),
    ('non_std:update', '更新非标库存', 'non_std', 'update'),
    ('non_std:delete', '删除非标库存', 'non_std', 'delete'),
    ('non_std:convert', '转换非标库存', 'non_std', 'convert'),
    ('non_std:read', '查看非标库存', 'non_std', 'read'),
    ('audit:read', '查看审计日志', 'audit', 'read')
ON CONFLICT (code) DO UPDATE SET
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
ON CONFLICT DO NOTHING;
