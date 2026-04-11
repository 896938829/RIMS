"""配置加载模块。

职责：
    从 `.env`（找不到则回退 `.env.example`）读取运行时配置，暴露为模块级常量。
    业务代码应直接 `from core.config import CONFIG` 使用。
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional

from dotenv import load_dotenv


@dataclass(frozen=True)
class Config:
    """运行时配置快照，字段含义见下方注释。"""

    # RIMS Go 后端基础 URL，包含 /api/v1 前缀
    base_url: str
    # 默认登录用户名（通常是 seed 出来的 admin）
    admin_user: str
    # 默认登录密码
    admin_password: str
    # 默认的 X-Warehouse-ID；None 表示不发送该头，由后端使用用户默认仓库
    default_warehouse_id: Optional[int]
    # 日志文件输出目录
    log_dir: str


def _locate_env_file() -> Path:
    """查找 .env 文件位置。

    规则：
    1) 当前工作目录下的 .env
    2) 本项目根目录下的 .env
    3) 本项目根目录下的 .env.example（仅示例，确保首次运行也能启动）
    """
    here = Path(__file__).resolve().parent.parent  # rims-py-validator/
    candidates = [
        Path.cwd() / ".env",
        here / ".env",
        here / ".env.example",
    ]
    for p in candidates:
        if p.is_file():
            return p
    # 都找不到时返回第一个候选，dotenv 静默失败即可
    return candidates[0]


def load_config() -> Config:
    """装载配置；每次调用都会重新读取 .env，方便测试中覆盖。"""
    env_path = _locate_env_file()
    load_dotenv(dotenv_path=env_path, override=False)

    # 将字符串形式的仓库 ID 转整型；空串视为未设置
    raw_wid = os.getenv("DEFAULT_WAREHOUSE_ID", "").strip()
    wid: Optional[int] = int(raw_wid) if raw_wid else None

    return Config(
        base_url=os.getenv("BASE_URL", "http://localhost:8080/api/v1").rstrip("/"),
        admin_user=os.getenv("ADMIN_USER", "admin"),
        admin_password=os.getenv("ADMIN_PASSWORD", "admin123"),
        default_warehouse_id=wid,
        log_dir=os.getenv("LOG_DIR", "./logs"),
    )


# 模块级单例配置，import 即用
CONFIG: Config = load_config()
