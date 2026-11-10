# GitHub 分支与历史记录

日常开发从 `main` 建立任务分支；已合并且没有新增产品提交的远端分支在核对后退役，历史内容通过 main、PR 和固定 Git 提交追溯。删除远端名称不代表删除本机 Worktree、功能代码或任务记录。

2026-11-10 本次核对 GitHub 共 65 条分支：63 条已完成历史分支列入退役计划，`main` 保留；`docs/shared-rate-limit-deployment` 对应 [PR #75](https://github.com/zewbby/SpeechFUL/pull/75) 仍未合并，含 operator SETNX 的未接纳修正，保留。合并后额外回执先核对并归集，再删除远端名称；本次工作分支也在自身 PR 合并后退役。实际执行结果、期望 SHA 和名单集中见[唯一任务记录](../tasks/STALE-FILE-CLEANUP-20261110.md)，不把这里的快照当作实时 GitHub 清单。

## 已退役的分支过程文档

77 份分支说明和合并回执已从当前目录退役，原文保存在[清理前的固定提交](https://github.com/zewbby/SpeechFUL/tree/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/branches)。这些记录描述当时的工作目录、分支和验收范围，不代表现在的状态。

当前工作查看[任务记录](../tasks/)和[近期工作](../RECENT_WORK.md)；开发规则见 [CONTRIBUTING](../../CONTRIBUTING.md)。不再按分支新增进度文档，也不恢复根 README 已移除的“分支一览”。完整历史入口见[历史追溯](../archive/README.md)。
