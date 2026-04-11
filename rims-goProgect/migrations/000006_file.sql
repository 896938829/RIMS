-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: file_attachments (文件附件元数据表)

-- 文件附件表：存储商品图片、单据附件、导入模板、导出结果等文件的元数据。
-- 实际对象按 v1 方案存放在本地磁盘 (cfg.UploadDir)；后续可切换对象存储，不影响此表结构。
CREATE TABLE IF NOT EXISTS file_attachments (
    id            BIGSERIAL     PRIMARY KEY,
    business_type VARCHAR(32)   NOT NULL,
    business_id   BIGINT,
    object_key    VARCHAR(512)  NOT NULL,
    file_url      VARCHAR(1024) NOT NULL,
    original_name VARCHAR(255)  NOT NULL,
    file_size     BIGINT        NOT NULL DEFAULT 0,
    file_hash     VARCHAR(64)   NOT NULL DEFAULT '',
    mime_type     VARCHAR(128)  NOT NULL DEFAULT '',
    is_public     BOOLEAN       NOT NULL DEFAULT FALSE,
    created_by    BIGINT        NOT NULL DEFAULT 0,
    updated_by    BIGINT        NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

-- 按业务归属查询 (商品图、单据附件)
CREATE INDEX IF NOT EXISTS idx_file_business
    ON file_attachments (business_type, business_id)
    WHERE deleted_at IS NULL;

-- object_key 唯一，防止同一对象被重复登记
CREATE UNIQUE INDEX IF NOT EXISTS idx_file_object_key
    ON file_attachments (object_key)
    WHERE deleted_at IS NULL;

-- hash 索引：供未来去重接入
CREATE INDEX IF NOT EXISTS idx_file_hash
    ON file_attachments (file_hash)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_file_deleted_at ON file_attachments (deleted_at);
