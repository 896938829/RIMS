"""跨流程共享的登录态。

当某个流程完成登录后，将 token/user 信息写入这里，
后续流程可以直接读取，避免每个命令都重新登录。

注意：这是进程内单例，跨 CLI 调用不会持久化。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Optional


@dataclass
class SessionState:
    """当前登录会话的运行时状态。"""

    # JWT token 明文
    token: Optional[str] = None
    # 登录用户的基本信息（后端返回的 user 对象原样存放）
    user: dict = field(default_factory=dict)
    # 角色代码：admin / user
    role_code: str = ""
    # 当前选中的仓库 ID
    warehouse_id: Optional[int] = None

    # ---------- 便捷判断 ----------

    def is_admin(self) -> bool:
        """是否具备管理员权限。"""
        return self.role_code == "admin"

    def is_logged_in(self) -> bool:
        """是否已登录（即 token 非空）。"""
        return bool(self.token)


# 进程级单例，业务流程直接 from core.session import SESSION
SESSION = SessionState()
