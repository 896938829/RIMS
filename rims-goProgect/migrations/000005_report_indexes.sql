-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

-- Migration: compound indexes for the report & analytics module
-- 报表分析模块所需的复合索引。全部为部分索引 (WHERE deleted_at IS NULL)，
-- 匹配所有报表查询对软删除行的统一排除条件，保持索引紧凑。

-- inventory_transactions: 支持销售趋势、周转率、滞销分析
-- 过滤条件形如 warehouse_id = ? AND doc_type = 2 AND operated_at BETWEEN ? AND ?
-- 列顺序遵循等值谓词在前、范围谓词在后的 B-tree 最佳实践。
CREATE INDEX IF NOT EXISTS idx_inv_txn_wh_type_op
    ON inventory_transactions (warehouse_id, doc_type, operated_at)
    WHERE deleted_at IS NULL;

-- inventory_transactions: 支持按商品维度的时间窗口聚合 (周转、滞销明细)
CREATE INDEX IF NOT EXISTS idx_inv_txn_wh_prod_op
    ON inventory_transactions (warehouse_id, product_id, operated_at)
    WHERE deleted_at IS NULL;

-- documents: 支持销售统计/趋势/排行 (warehouse + status=2 + doc_type=2 + 日期范围)
CREATE INDEX IF NOT EXISTS idx_documents_wh_status_type_op
    ON documents (warehouse_id, status, doc_type, operated_at)
    WHERE deleted_at IS NULL;

-- document_lines: 加速销售聚合 JOIN + GROUP BY product_id
CREATE INDEX IF NOT EXISTS idx_doc_lines_doc_prod
    ON document_lines (document_id, product_id)
    WHERE deleted_at IS NULL;
