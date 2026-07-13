-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: seed permissions for idempotent offline synchronization routes

INSERT INTO permissions (code, name, resource, action)
VALUES
    ('document:create', '创建单据', 'document', 'create'),
    ('document:complete', '完成单据', 'document', 'complete'),
    ('stocktake:confirm', '确认盘点', 'stocktake', 'confirm'),
    ('stocktake:settle', '结算盘点', 'stocktake', 'settle'),
    ('file:upload', '上传文件', 'file', 'upload'),
    ('file:replace', '替换文件', 'file', 'replace')
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
  AND p.code IN (
      'document:create',
      'document:complete',
      'stocktake:confirm',
      'stocktake:settle',
      'file:upload',
      'file:replace'
  )
ON CONFLICT DO NOTHING;
