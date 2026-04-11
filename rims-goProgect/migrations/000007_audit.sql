-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: audit_logs (审计日志表)

-- 审计日志表：记录所有关键写操作的操作者、仓库、资源、结果、来源终端和时间。
-- 仅由服务层 (audit.AuditService.Log) 在业务事务内写入；无对外写接口。
-- 保留软删字段以便后续按保留期归档 / 清理。
CREATE TABLE IF NOT EXISTS audit_logs (
    id            BIGSERIAL     PRIMARY KEY,
    trace_id      VARCHAR(64)   NOT NULL DEFAULT '',
    user_id       BIGINT        NOT NULL DEFAULT 0,
    username      VARCHAR(64)   NOT NULL DEFAULT '',
    role_code     VARCHAR(32)   NOT NULL DEFAULT '',
    warehouse_id  BIGINT,
    action        VARCHAR(32)   NOT NULL,
    resource      VARCHAR(64)   NOT NULL,
    resource_id   BIGINT,
    doc_no        VARCHAR(64)   NOT NULL DEFAULT '',
    description   VARCHAR(255)  NOT NULL DEFAULT '',
    details       JSONB         NOT NULL DEFAULT '{}'::jsonb,
    ip_address    VARCHAR(45)   NOT NULL DEFAULT '',
    user_agent    VARCHAR(255)  NOT NULL DEFAULT '',
    result        VARCHAR(16)   NOT NULL DEFAULT 'success',
    error_code    INTEGER       NOT NULL DEFAULT 0,
    error_msg     VARCHAR(255)  NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

-- 按操作人 + 时间倒序检索（最常见的“查某人最近操作”路径）
CREATE INDEX IF NOT EXISTS idx_audit_user_time
    ON audit_logs (user_id, created_at DESC, result)
    WHERE deleted_at IS NULL;

-- 按仓库 + 时间检索，跨仓对比审计时使用
CREATE INDEX IF NOT EXISTS idx_audit_wh_time
    ON audit_logs (warehouse_id, created_at DESC)
    WHERE deleted_at IS NULL AND warehouse_id IS NOT NULL;

-- 按资源定位某对象的完整历史
CREATE INDEX IF NOT EXISTS idx_audit_res
    ON audit_logs (resource, resource_id)
    WHERE deleted_at IS NULL;

-- 单据号快查 (§3.7 要求之一)
CREATE INDEX IF NOT EXISTS idx_audit_doc_no
    ON audit_logs (doc_no)
    WHERE deleted_at IS NULL AND doc_no <> '';

-- 按动作筛选 (login/complete/delete 等)
CREATE INDEX IF NOT EXISTS idx_audit_action
    ON audit_logs (action)
    WHERE deleted_at IS NULL;

-- traceId 关联：同一请求链路的审计串联
CREATE INDEX IF NOT EXISTS idx_audit_trace
    ON audit_logs (trace_id)
    WHERE deleted_at IS NULL AND trace_id <> '';

-- 通用时间倒序兜底索引
CREATE INDEX IF NOT EXISTS idx_audit_created_at
    ON audit_logs (created_at DESC)
    WHERE deleted_at IS NULL;

-- 软删索引 (与其它模块一致)
CREATE INDEX IF NOT EXISTS idx_audit_deleted_at ON audit_logs (deleted_at);
