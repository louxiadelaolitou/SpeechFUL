# REPOSITORY-RECOVERY-20261115 — 重建源码仓库并整理当前文档

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／仓库：`codex/chore/recovery-docs-20261115`；`D:/SpeechFUL/.worktrees/recovery-docs-20261115`；基线为 `D:/SpeechFUL` 本地 `main` 的 `f5127dc9566daf545afe9c2ce26cd233a4bc54dd`。新建私有仓库 `zewbby/SpeechFUL-Develop`；原 `zewbby/SpeechFUL` 是不同历史，未修改。
- 授权依据与边界：用户要求以截图中的 `D:/SpeechFUL` 当前 `main` 为基准创建新仓库，并在推送前检查和移除过时、冗余的 `docs` 材料。用户随后确认只清理新仓库当前目录，保留旧提交历史。授权覆盖本次文档整理及向新仓库交付主线，不扩展为其他分支、PR 或正式部署。
- 需求确认：旧记录、候选截图和无现行依赖的阶段证据从当前树退役；安全契约、现行运行说明、任务记录和测试校验的原始归档保留。退役文件仍可在固定基线提交读取。无未决取舍。
- 更新日期：2026-11-15。
- 状态：文档实现 已完成；本地验证 PASS；新仓库主线已建立并核对；发布 未发布。

## 目标与范围

新仓库的 `main` 以指定本地提交为基线，只增加文档清理提交；不把原 `zewbby/SpeechFUL` 的独立历史合并进来。清理过时的 release 候选回执、讨论附件、被替代图稿和无现行引用的阶段材料，保留可追溯的旧 Git 历史。更新文档入口与可由新仓库主线历史解析的固定提交链接，不重写原始 JSON 或归档字节。

## 接口与依赖

主线包含 Git LFS 字体对象；推送时须核对 LFS 上传与远端 `main` SHA。旧仓库删除后，旧 PR、Actions 和 Release 页面不会随 Git 提交重建；任务记录仅保留原时点标识。

## 验收标准

- 新仓库为私有，远端 `main` 与本轮文档清理提交一致；`D:/SpeechFUL` 原工作区及其他 Worktree 的文件和分支保持原状。
- 当前 `docs` 不再包含本轮确认退役的文件；现行本地链接、测试输入和固定提交引用可解析，清理文件在基线 `f5127dc` 中可追溯。
- 文档与协作检查通过；不把本次文档验证记作产品、设备、生产验收。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 新仓库 | GitHub `zewbby/SpeechFUL-Develop`；`git ls-remote --symref` | PASS：创建私有仓库；初次推送依用户补充要求中止，清理完成后将 `86f9bad0659d31ce664eb137457b2b3436ff90fe` 推送为初始 `main`，远端 HEAD 与 `main` 均为该 SHA；Git LFS 上传完成。 |
| 本地工作区 | `D:/SpeechFUL`；`git status`、`git remote -v`、`git branch -vv` | PASS：原 `main` 干净后仅快进到 `86f9bad`，新私有仓库成为 `origin` 且 `main` 跟踪 `origin/main`；另一份 `zewbby/SpeechFUL` 保留为 `public-speechful`，其他 Worktree 的文件与分支 HEAD 未改动，既有上游跟踪随远端重命名保留。 |
| 文档整理 | `f5127dc`＋本分支差异，Windows PowerShell／Python | PASS：从当前树退役 78 个文件；更新预发布手册和入口，修复 281 处旧固定提交链接。其余 54 份被改动文档仅替换仓库 URL；原始归档字节不变。 |
| 链接与历史 | 同上；遍历当前非原始归档 Markdown，`git cat-file -e` 检查固定提交路径 | PASS：848 个本地链接、284 个新仓库固定提交链接均可解析；78 个退役文件均来自 `f5127dc` 基线。原始归档保留其原时点相对链接，不改写字节。 |
| 协作与契约 | Windows／Python；`python -B -X utf8 -m unittest discover -s tools/tests -p test_collaboration.py -v`、`test_community_text_contract.py`、`git diff --check` | PASS：协作 19 项、文字归档契约 10 项、差异空白检查。当前变更仅涉及文档与 `infra/README.md`。 |
| 产品运行 | 本轮仅文档与仓库迁移 | NOT_RUN：不据此提升原 SQL、设备、CI 或生产结论。 |

## 阻碍与下一步

本次记录补充不改变已验证的文档内容；旧平台 PR、Issue、Actions 和 Release 元数据不属于 Git 提交恢复范围。后续开发以新仓库 `origin/main` 建立任务分支，并重新配置主线保护与适用检查。
