"""统一 HTTP 客户端：所有业务流程访问后端都必须经过这里。

核心职责：
1. 自动附带 `Authorization: Bearer <token>` 与 `X-Warehouse-ID: <id>` 头；
2. 每次 **请求发起前** 打印 `[REQ] METHOD URL headers body_summary`；
3. 每次 **响应返回后** 打印 `[RSP] status code=... message=... traceId=...`；
4. 自动解包后端响应 envelope `{code, message, data, traceId}`，
   在 `code != 0` 时抛 `APIError`，在 HTTP 非 2xx 时抛 `APIError`；
5. 对请求体中的 `password` 字段进行 `***` 脱敏，避免密码写入日志文件。

业务代码调用示例：
    client.post("/auth/login", json={"username": "admin", "password": "x"}, scoped=False)
    client.get("/documents", params={"page": 1}, scoped=True)
"""

from __future__ import annotations

import json as _json
from typing import Any, Mapping, Optional

import requests

from .logger import get_logger


class APIError(RuntimeError):
    """后端返回的业务错误或网络错误。"""

    def __init__(self, code: int, message: str, *, http_status: int = 0, trace_id: str = ""):
        super().__init__(f"[code={code}] {message}")
        self.code = code
        self.message = message
        self.http_status = http_status
        self.trace_id = trace_id


# 请求日志中体积过大的字段会被截断到这个长度，避免刷屏
_BODY_PREVIEW_LIMIT = 512


def _mask_sensitive(body: Any) -> Any:
    """对请求体做最小脱敏：password 字段替换成 ***。"""
    if not isinstance(body, Mapping):
        return body
    masked = dict(body)
    for key in list(masked.keys()):
        if "password" in key.lower():
            masked[key] = "***"
    return masked


def _summarize(body: Any) -> str:
    """把请求/响应体转成日志可读的短字符串。"""
    if body is None:
        return "-"
    try:
        text = _json.dumps(body, ensure_ascii=False)
    except (TypeError, ValueError):
        text = str(body)
    if len(text) > _BODY_PREVIEW_LIMIT:
        return text[:_BODY_PREVIEW_LIMIT] + f"... (+{len(text) - _BODY_PREVIEW_LIMIT}B)"
    return text


class APIClient:
    """RIMS 后端 HTTP 客户端封装。"""

    def __init__(self, base_url: str):
        # 去掉末尾斜杠，拼路径时统一用 "/xxx"
        self.base_url = base_url.rstrip("/")
        self._token: Optional[str] = None
        self._warehouse_id: Optional[int] = None
        self._session = requests.Session()
        self._log = get_logger("client")

    # ---------- 设置态 ----------

    def set_token(self, token: Optional[str]) -> None:
        """登录成功后调用；None 表示退出登录。"""
        self._token = token
        self._log.debug("JWT token 已%s", "更新" if token else "清空")

    def set_warehouse(self, warehouse_id: Optional[int]) -> None:
        """设置/清除当前仓库作用域头。"""
        self._warehouse_id = warehouse_id
        self._log.debug("当前仓库 ID 已设置为 %s", warehouse_id)

    # ---------- 核心请求 ----------

    def request(
        self,
        method: str,
        path: str,
        *,
        json: Any = None,
        params: Optional[Mapping[str, Any]] = None,
        files: Any = None,
        data: Any = None,
        scoped: bool = True,
        extra_headers: Optional[Mapping[str, str]] = None,
    ) -> Any:
        """发起一次请求并返回后端 envelope 中的 `data` 字段。

        参数：
            method: HTTP 方法
            path: 相对路径，形如 "/auth/login"；不需要包含 base_url
            json: JSON 请求体
            params: URL 查询参数
            files: multipart 文件字段
            data: 表单/原始 body（和 files 一起用）
            scoped: 是否附加 X-Warehouse-ID 头（仓库作用域路由必须 True）
            extra_headers: 额外自定义头

        返回：
            业务 `data` 字段（可能是 dict / list / None）

        异常：
            APIError — 网络错误或 `code != 0`
        """
        url = f"{self.base_url}{path if path.startswith('/') else '/' + path}"
        headers: dict[str, str] = {}
        if self._token:
            headers["Authorization"] = f"Bearer {self._token}"
        if scoped and self._warehouse_id is not None:
            headers["X-Warehouse-ID"] = str(self._warehouse_id)
        if extra_headers:
            headers.update(extra_headers)

        # ---- 请求前日志 ----
        has_token = "yes" if self._token else "no"
        has_wh = headers.get("X-Warehouse-ID", "-")
        body_for_log = _summarize(_mask_sensitive(json)) if json is not None else "-"
        self._log.info(
            "[REQ] %s %s token=%s warehouse=%s params=%s body=%s",
            method.upper(),
            url,
            has_token,
            has_wh,
            _summarize(params),
            body_for_log,
        )

        # ---- 实际发起请求 ----
        try:
            resp = self._session.request(
                method=method.upper(),
                url=url,
                json=json if files is None else None,
                params=params,
                files=files,
                data=data,
                headers=headers,
                timeout=30,
            )
        except requests.RequestException as e:
            # 网络层错误：直接封装成 APIError，并输出日志
            self._log.error("[ERR] %s %s 网络异常: %s", method.upper(), url, e)
            raise APIError(code=-1, message=f"网络异常: {e}") from e

        # ---- 解析响应 envelope ----
        try:
            payload = resp.json()
        except ValueError:
            self._log.error(
                "[RSP] %s %s status=%s body 非 JSON: %s",
                method.upper(),
                url,
                resp.status_code,
                resp.text[:_BODY_PREVIEW_LIMIT],
            )
            raise APIError(
                code=-2,
                message=f"响应非 JSON (HTTP {resp.status_code})",
                http_status=resp.status_code,
            )

        code = payload.get("code", -3)
        message = payload.get("message", "")
        trace_id = payload.get("traceId", "")
        data_field = payload.get("data")

        # ---- 响应后日志 ----
        self._log.info(
            "[RSP] %s %s status=%s code=%s message=%s traceId=%s",
            method.upper(),
            url,
            resp.status_code,
            code,
            message,
            trace_id,
        )
        self._log.debug("[RSP.body] %s", _summarize(data_field))

        # ---- 业务错误校验 ----
        if resp.status_code >= 400 or code != 0:
            raise APIError(
                code=code,
                message=message or f"HTTP {resp.status_code}",
                http_status=resp.status_code,
                trace_id=trace_id,
            )

        return data_field

    # ---------- 便捷方法 ----------

    def get(self, path: str, **kw: Any) -> Any:
        return self.request("GET", path, **kw)

    def post(self, path: str, **kw: Any) -> Any:
        return self.request("POST", path, **kw)

    def put(self, path: str, **kw: Any) -> Any:
        return self.request("PUT", path, **kw)

    def delete(self, path: str, **kw: Any) -> Any:
        return self.request("DELETE", path, **kw)
