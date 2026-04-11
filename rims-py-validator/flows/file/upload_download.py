"""文件上传/下载流程（PRD 第 9 章 - 文件附件）。

预期步骤：
    1) POST /files/upload 上传 product_image（public 类型），取回 fileUrl
    2) GET 静态 URL /uploads/xxx 断言 200
    3) POST /files/upload 上传 document_attachment（private）
    4) GET /files/:id/download 通过代理下载
    5) DELETE /files/:id 删除（上传者或 admin）
    6) 上传超出 MaxUploadMB 应返回业务错误
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现文件上传（多种类型）+ 下载 + 删除 + 大小限制校验
def run(client: APIClient) -> None:
    not_implemented("file.upload_download")
