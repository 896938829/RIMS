"""非标库存转换流程（PRD 第 6 章 - 非标商品处理，admin-only）。

预期步骤：
    1) POST /non-std-inventory 创建非标条目
    2) GET /non-std-inventory 列表校验
    3) POST /non-std-inventory/:id/convert 部分/全部转标
    4) 转换后 GET /inventory 应能看到对应标准 SKU
    5) 普通用户访问应 403（用 expect_error 覆盖）
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现非标创建 → 转标 → 权限校验
def run(client: APIClient) -> None:
    not_implemented("inventory.non_std_convert")
