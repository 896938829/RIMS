"""断言工具集：统一的校验/期望 API。

业务流程用这些小函数做"肉眼可见的校验"，失败时抛 AssertionError
或 APIError，CLI 层会把它转成非零退出码。
"""

from __future__ import annotations

from typing import Any, Callable, Optional

from .client import APIError


def assert_eq(actual: Any, expected: Any, label: str) -> None:
    """断言相等；失败时抛 AssertionError 并标注字段名。"""
    if actual != expected:
        raise AssertionError(f"{label} 期望 {expected!r}，实际 {actual!r}")


def assert_in(key: str, container: Any, label: str) -> None:
    """断言 key 在容器内，常用于校验返回字典是否包含某字段。"""
    if key not in container:
        raise AssertionError(f"{label} 缺少字段 {key!r}，实际内容={container!r}")


def assert_page(data: Any, *, min_total: int = 0) -> None:
    """分页响应断言：验证形状 `{items, total, page, pageSize}`。"""
    if not isinstance(data, dict):
        raise AssertionError(f"分页响应不是对象：{data!r}")
    for field_name in ("items", "total", "page", "pageSize"):
        if field_name not in data:
            raise AssertionError(f"分页响应缺少字段 {field_name}：{data!r}")
    if data["total"] < min_total:
        raise AssertionError(
            f"分页 total 期望 >= {min_total}，实际 {data['total']}"
        )


def expect_error(
    fn: Callable[[], Any],
    *,
    code: Optional[int] = None,
    http_status: Optional[int] = None,
) -> APIError:
    """期望调用失败；成功时抛 AssertionError。

    用法：
        expect_error(lambda: client.get("/admin-only"), http_status=403)
    """
    try:
        fn()
    except APIError as e:
        if code is not None and e.code != code:
            raise AssertionError(
                f"期望业务错误码 {code}，实际 {e.code}（message={e.message}）"
            )
        if http_status is not None and e.http_status != http_status:
            raise AssertionError(
                f"期望 HTTP {http_status}，实际 {e.http_status}"
            )
        return e
    raise AssertionError("期望调用失败，但它成功了")
