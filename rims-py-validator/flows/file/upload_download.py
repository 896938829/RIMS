"""文件上传/下载流程（PRD 第 9 章 - 文件附件）。

对应 PRD 摘录（见 docs/产品需求文档.md:29,86,238）：
    - "附件上传"：业务单据可附带附件；
    - "商品图片上传"：商品档案公开图片；
    - "附件、报表与审计数据"：文件元数据需入库。

后端实现约束（rims-goProgect/internal/modules/file/）：
    - 允许扩展名白名单 + 单文件最大字节限制（由 .env 的 MaxUploadMB 决定，默认 10MB）
    - businessType=product_image → isPublic=true，fileUrl 直接给 /uploads/YYYY/MM/<hex>.<ext>
    - businessType=doc_attachment 等私有类型 → fileUrl 是 /api/v1/files/:id/download（需鉴权）
    - 删除规则：仅上传者本人或 admin；非 uploader 非 admin → 403
    - /api/v1/files/* 路由鉴权但 *不* 走仓库作用域（files.Group + authMw，未用 WarehouseScope）

预期业务步骤：
    1) admin 上传 product_image（public 图片）→ 校验 isPublic / fileUrl / hash
    2) 通过静态 /uploads/* 匿名可读（Client.download_raw 不带 token 命中 200）
    3) admin 上传 doc_attachment（private PDF）→ fileUrl 为 /api/v1/files/{id}/download
    4) GET /files/{id}/download 需要 token，返回原始字节 + Content-Disposition
    5) 列表过滤（businessType=doc_attachment）与详情（admin 能看到 objectKey）
    6) 超大文件 / 非法扩展名 / 未提供文件 三条失败路径
    7) 权限边界：创建两个普通用户 A/B；A 上传私有文件，B 尝试删除 → 期望 403
    8) admin 清理所有测试数据并验证软删后 404

测试原则：模拟用户操作，寻找后端漏洞。所有"意外成功"的权限穿透记录到 vulnerabilities。
"""

from __future__ import annotations

import time
from typing import Callable, List, Optional

from core.assertions import assert_eq, assert_in, assert_page, expect_error
from core.client import APIClient, APIError
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.file.upload")


# --------------- 测试数据构造 ---------------


def _png_bytes() -> bytes:
    """构造一张最小合法的 1x1 PNG 字节串。

    不依赖 Pillow，直接拼二进制：PNG 签名 + IHDR + IDAT + IEND。
    后端只校验扩展名 + MIME 嗅探前 512 字节，这个字节串足够被识别为 image/png。
    """
    # 标准的 1x1 透明 PNG（来源：常见最小 PNG 模板，字节固定）
    return (
        b"\x89PNG\r\n\x1a\n"
        b"\x00\x00\x00\rIHDR"
        b"\x00\x00\x00\x01\x00\x00\x00\x01"
        b"\x08\x06\x00\x00\x00\x1f\x15\xc4\x89"
        b"\x00\x00\x00\rIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01"
        b"\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82"
    )


def _pdf_bytes() -> bytes:
    """构造一个最小合法的 PDF（带 %PDF- 魔数，能让 http.DetectContentType 返回 application/pdf）。"""
    # %PDF-1.4 头 + 一个空 trailer，够通过 MIME 嗅探即可，不需要真正能被阅读
    return (
        b"%PDF-1.4\n"
        b"1 0 obj<<>>endobj\n"
        b"trailer<<>>\n"
        b"%%EOF\n"
    )


def _oversize_bytes(mb: int = 11) -> bytes:
    """构造超出 10MB 限制的字节串（默认 11MB）。"""
    # 用单字节填充；PNG 扩展名但内容垃圾 —— 目的只是触发体积限制
    return b"x" * (mb * 1024 * 1024 + 1)


def _unique_suffix() -> str:
    """用毫秒时间戳生成唯一后缀，避免多次运行文件名碰撞。"""
    return str(int(time.time() * 1000))


# --------------- 辅助：权限探测 ---------------


def _probe_permission(label: str, fn: Callable[[], object]) -> bool:
    """期望调用被 403 拦截；若未被拦截记录为漏洞。

    返回 True 表示拦截正常，False 表示存在权限漏洞（但不抛异常）。
    """
    try:
        expect_error(fn, http_status=403)
        _log.info("权限拦截正常：%s", label)
        return True
    except AssertionError:
        _log.warning("⚠ 权限漏洞: %s — 非 admin 非 uploader 未被拦截 (期望 HTTP 403)", label)
        return False


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色 code 反查角色 id，用于创建测试用户。"""
    _log.info("查询角色列表寻找 code=%s", code)
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            _log.info("找到角色：id=%s code=%s", r["id"], r["code"])
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _safe_delete_user(client: APIClient, user_id: Optional[int]) -> None:
    """best-effort 删除用户，吞掉 404；供 finally 收尾使用。"""
    if not user_id:
        return
    try:
        client.delete(f"/users/{user_id}", scoped=False)
        _log.info("清理测试用户：id=%s", user_id)
    except APIError as e:
        _log.warning("清理用户 id=%s 失败（已忽略）：%s", user_id, e.message)


def _safe_delete_file(client: APIClient, file_id: Optional[int]) -> None:
    """best-effort 删除文件，吞掉 404/403；供 finally 收尾使用。"""
    if not file_id:
        return
    try:
        client.delete(f"/files/{file_id}", scoped=False)
        _log.info("清理测试文件：id=%s", file_id)
    except APIError as e:
        _log.warning("清理文件 id=%s 失败（已忽略）：%s", file_id, e.message)


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行文件上传/下载全流程。

    前置：调用方已以 admin 身份登录（main.py._register_todo 保证）。
    """
    vulnerabilities: List[str] = []
    # 这些变量在 finally 里用于清理，初始化为 None 防 UnboundLocalError
    public_file_id: Optional[int] = None
    private_file_id: Optional[int] = None
    user_a_id: Optional[int] = None
    user_b_id: Optional[int] = None
    ab_file_id: Optional[int] = None

    suffix = _unique_suffix()
    admin_user = CONFIG.admin_user
    admin_pwd = CONFIG.admin_password

    try:
        # ========== 第 1 步：admin 上传 public 图片 ==========
        png_body = _png_bytes()
        png_name = f"probe_public_{suffix}.png"
        _log.info("开始上传 public 图片：filename=%s size=%dB businessType=product_image",
                  png_name, len(png_body))
        public_resp = client.post(
            "/files/upload",
            files={"file": (png_name, png_body, "image/png")},
            data={"businessType": "product_image"},
            scoped=False,
        )
        for k in ("id", "fileUrl", "isPublic", "fileHash", "fileSize", "mimeType"):
            assert_in(k, public_resp, "public 上传响应")
        assert_eq(public_resp["isPublic"], True, "public 上传.isPublic")
        assert_eq(public_resp["fileSize"], len(png_body), "public 上传.fileSize")
        if not public_resp["fileUrl"].startswith("/uploads/"):
            raise AssertionError(
                f"public fileUrl 期望以 /uploads/ 开头，实际={public_resp['fileUrl']!r}"
            )
        if not public_resp["fileHash"]:
            raise AssertionError("public 上传 fileHash 为空")
        public_file_id = public_resp["id"]
        public_url = public_resp["fileUrl"]
        _log.info("public 上传成功：id=%s fileUrl=%s hash=%s",
                  public_file_id, public_url, public_resp["fileHash"][:12])

        # ========== 第 2 步：public 静态 URL 匿名可访问 ==========
        # 构造一个不带 token 的临时 client，验证静态路由真的是公开的
        _log.info("匿名下载 public 静态 URL：%s", public_url)
        anon_client = APIClient(base_url=CONFIG.base_url)  # 没有 set_token
        body, headers = anon_client.download_raw(public_url, scoped=False)
        assert_eq(len(body), len(png_body), "public 下载字节数")
        if not body.startswith(b"\x89PNG"):
            raise AssertionError(f"public 下载内容不是 PNG，前 8 字节={body[:8]!r}")
        _log.info("public 静态下载通过：字节数=%d content-type=%s",
                  len(body), headers.get("Content-Type", "-"))

        # ========== 第 3 步：admin 上传 private PDF（doc_attachment） ==========
        pdf_body = _pdf_bytes()
        pdf_name = f"probe_private_{suffix}.pdf"
        _log.info("开始上传 private PDF：filename=%s size=%dB businessType=doc_attachment",
                  pdf_name, len(pdf_body))
        private_resp = client.post(
            "/files/upload",
            files={"file": (pdf_name, pdf_body, "application/pdf")},
            data={"businessType": "doc_attachment"},
            scoped=False,
        )
        assert_eq(private_resp["isPublic"], False, "private 上传.isPublic")
        assert_eq(private_resp["fileSize"], len(pdf_body), "private 上传.fileSize")
        private_file_id = private_resp["id"]
        expected_dl = f"/api/v1/files/{private_file_id}/download"
        assert_eq(private_resp["fileUrl"], expected_dl, "private fileUrl")
        # admin 身份应当能看到 objectKey
        if not private_resp.get("objectKey"):
            _log.warning("⚠ 可能漏洞：admin 查看 private 上传响应缺少 objectKey 字段")
            vulnerabilities.append("admin 上传响应未返回 objectKey")
        _log.info("private 上传成功：id=%s fileUrl=%s", private_file_id, private_resp["fileUrl"])

        # ========== 第 4 步：鉴权下载 private 文件 ==========
        _log.info("鉴权下载 private 文件：GET /files/%s/download", private_file_id)
        body, headers = client.download_raw(f"/files/{private_file_id}/download", scoped=False)
        assert_eq(len(body), len(pdf_body), "private 下载字节数")
        if not body.startswith(b"%PDF"):
            raise AssertionError(f"private 下载内容不是 PDF，前 8 字节={body[:8]!r}")
        cd = headers.get("Content-Disposition", "")
        if "filename" not in cd:
            raise AssertionError(f"Content-Disposition 未携带 filename: {cd!r}")
        _log.info("private 下载通过：字节数=%d Content-Disposition=%s", len(body), cd)

        # 不带 token 下载 private：应当 401（进一步确认私有路由是真的私有）
        _log.info("匿名访问 private 下载，期望被拦截")
        try:
            expect_error(
                lambda: anon_client.download_raw(
                    f"/files/{private_file_id}/download", scoped=False
                ),
                http_status=401,
            )
            _log.info("匿名访问 private 下载被 401 拦截，符合预期")
        except AssertionError:
            _log.warning("⚠ 权限漏洞：匿名可下载私有文件 /files/%s/download", private_file_id)
            vulnerabilities.append("匿名可下载私有文件")

        # ========== 第 5 步：列表过滤 + 详情 ==========
        _log.info("列表查询 businessType=doc_attachment")
        page = client.get(
            "/files",
            params={"businessType": "doc_attachment", "page": 1, "pageSize": 50},
            scoped=False,
        )
        assert_page(page, min_total=1)
        found = any(item.get("id") == private_file_id for item in page["list"])
        if not found:
            raise AssertionError(
                f"列表中未找到刚上传的 private 文件 id={private_file_id}"
            )
        _log.info("列表过滤通过：total=%s 包含目标 id=%s", page["total"], private_file_id)

        _log.info("查询 private 文件详情：GET /files/%s", private_file_id)
        detail = client.get(f"/files/{private_file_id}", scoped=False)
        assert_eq(detail["id"], private_file_id, "详情.id")
        assert_eq(detail["businessType"], "doc_attachment", "详情.businessType")
        assert_eq(detail["isPublic"], False, "详情.isPublic")
        # admin 应当能看到 objectKey（dto.go:48 gate）
        if not detail.get("objectKey"):
            _log.warning("⚠ 可能漏洞：admin 查看详情缺少 objectKey 字段")
            vulnerabilities.append("admin 查看详情未返回 objectKey")
        else:
            _log.info("admin 详情含 objectKey=%s", detail["objectKey"])

        # ========== 第 6 步：失败路径 ==========
        # 6a) 超限
        _log.info("失败路径：上传 11MB 文件，期望 HTTP 400")
        expect_error(
            lambda: client.post(
                "/files/upload",
                files={"file": (
                    f"oversize_{suffix}.png",
                    _oversize_bytes(11),
                    "image/png",
                )},
                data={"businessType": "product_image"},
                scoped=False,
            ),
            http_status=400,
        )
        _log.info("超限拦截通过")

        # 6b) 非法扩展名
        _log.info("失败路径：上传 .exe 文件，期望 HTTP 400")
        expect_error(
            lambda: client.post(
                "/files/upload",
                files={"file": (
                    f"bad_{suffix}.exe",
                    b"MZ\x90\x00" + b"\x00" * 32,
                    "application/octet-stream",
                )},
                data={"businessType": "other"},
                scoped=False,
            ),
            http_status=400,
        )
        _log.info("非法扩展名拦截通过")

        # 6c) 未提供文件
        _log.info("失败路径：POST /files/upload 不带 file 字段，期望 HTTP 400")
        expect_error(
            lambda: client.post(
                "/files/upload",
                # files 传一个不含 "file" 字段的占位，让 requests 仍用 multipart 编码
                files={"dummy": ("dummy.txt", b"x", "text/plain")},
                data={"businessType": "other"},
                scoped=False,
            ),
            http_status=400,
        )
        _log.info("未提供文件拦截通过")

        # ========== 第 7 步：权限边界 ==========
        _log.info("===== 开始权限边界测试（创建临时普通用户 A/B） =====")
        user_role_id = _get_role_id_by_code(client, "user")
        user_a_name = f"file_probe_a_{suffix}"
        user_b_name = f"file_probe_b_{suffix}"
        pwd_a = "ProbeA@12345"
        pwd_b = "ProbeB@12345"

        _log.info("创建用户 A：%s", user_a_name)
        a = client.post("/users", json={
            "username": user_a_name,
            "password": pwd_a,
            "realName": "文件探测A",
            "roleId": user_role_id,
        }, scoped=False)
        user_a_id = a["id"]

        _log.info("创建用户 B：%s", user_b_name)
        b = client.post("/users", json={
            "username": user_b_name,
            "password": pwd_b,
            "realName": "文件探测B",
            "roleId": user_role_id,
        }, scoped=False)
        user_b_id = b["id"]

        # 以 A 登录（会替换 client 上的 token/SESSION；files 路由不做仓库作用域，
        # 所以即便 A 没有绑定仓库也能调 /files/upload）
        _log.info("切换登录身份到 A")
        auth_login_flow.run(client, user_a_name, pwd_a)
        assert_eq(SESSION.role_code, "user", "A 角色")

        # A 上传一份私有文件
        ab_name = f"ab_probe_{suffix}.pdf"
        _log.info("A 上传 private 文件：%s", ab_name)
        ab_resp = client.post(
            "/files/upload",
            files={"file": (ab_name, _pdf_bytes(), "application/pdf")},
            data={"businessType": "doc_attachment"},
            scoped=False,
        )
        ab_file_id = ab_resp["id"]
        assert_eq(ab_resp["isPublic"], False, "A 上传.isPublic")
        # A 本人看自己上传的文件：detail 的 objectKey 应被屏蔽（非 admin）
        a_detail = client.get(f"/files/{ab_file_id}", scoped=False)
        if a_detail.get("objectKey"):
            _log.warning("⚠ 可能漏洞：普通用户 A 能看到 objectKey 字段")
            vulnerabilities.append("普通用户可见 objectKey")
        else:
            _log.info("普通用户 A 查看详情不含 objectKey（admin-only gate 生效）")

        # 切到 B
        _log.info("切换登录身份到 B")
        auth_login_flow.run(client, user_b_name, pwd_b)

        # B 尝试删除 A 的文件 → 期望 403
        if not _probe_permission(
            f"B 删除 A 上传的文件 DELETE /files/{ab_file_id}",
            lambda: client.delete(f"/files/{ab_file_id}", scoped=False),
        ):
            vulnerabilities.append("非 uploader 非 admin 可删除他人文件")

        # 额外探测：B 可以列表看到 A 的文件吗？（当前后端无 ACL 过滤，仅列出提示）
        _log.info("B 查看文件列表（提示：后端暂未按 uploader 过滤，列表结果仅供记录）")
        b_list = client.get(
            "/files",
            params={"businessType": "doc_attachment", "page": 1, "pageSize": 50},
            scoped=False,
        )
        b_sees_a = any(it.get("id") == ab_file_id for it in b_list.get("list", []))
        _log.info("B 列表里%s看到 A 的文件 id=%s", "能" if b_sees_a else "未", ab_file_id)

        # B 尝试匿名思路：直接 GET /files/{id}/download 用 B 的 token（登录用户 B 有效 token）
        # 这里仍然是有效 token —— 后端路由只要求 authMw 通过，没有 uploader 过滤
        # 这个探测只是确认"只要登录任何人都能下载私有文件"这个潜在弱点
        try:
            body_b, _ = client.download_raw(
                f"/files/{ab_file_id}/download", scoped=False
            )
            if len(body_b) > 0:
                _log.warning(
                    "⚠ 可能漏洞：用户 B 可下载用户 A 上传的私有文件（字节=%d）",
                    len(body_b),
                )
                vulnerabilities.append("任意登录用户可下载他人私有文件")
        except APIError as e:
            _log.info("B 下载 A 私有文件被拦截：code=%s http=%s", e.code, e.http_status)

        # 切回 admin 处理清理
        _log.info("切回 admin 进行清理")
        auth_login_flow.run(client, admin_user, admin_pwd)

        # ========== 第 8 步：admin 删除 private 文件 + 软删 404 验证 ==========
        _log.info("admin 删除 private 文件 id=%s", private_file_id)
        client.delete(f"/files/{private_file_id}", scoped=False)
        _log.info("删除成功，验证 GET 返回 404")
        expect_error(
            lambda: client.get(f"/files/{private_file_id}", scoped=False),
            http_status=404,
        )
        private_file_id = None  # 已清理，避免 finally 重复删
        _log.info("软删 404 验证通过")

        # ========== 报告漏洞汇总 ==========
        if vulnerabilities:
            _log.warning("本次发现 %d 个潜在漏洞：", len(vulnerabilities))
            for v in vulnerabilities:
                _log.warning("  - %s", v)
        else:
            _log.info("未发现已知类型漏洞")

    finally:
        # ---- 兜底清理 ----
        # 尝试恢复到 admin 身份以清理（无论中途切换到 A/B 与否）
        cleanup_ok = True
        try:
            if SESSION.role_code != "admin":
                _log.info("finally 中切回 admin 以清理资源")
                auth_login_flow.run(client, admin_user, admin_pwd)
        except APIError as e:
            _log.warning("finally 中 admin 重新登录失败（跳过清理）：%s", e.message)
            cleanup_ok = False

        if cleanup_ok:
            _safe_delete_file(client, ab_file_id)
            _safe_delete_file(client, private_file_id)
            _safe_delete_file(client, public_file_id)
            _safe_delete_user(client, user_a_id)
            _safe_delete_user(client, user_b_id)
