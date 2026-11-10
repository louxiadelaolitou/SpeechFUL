# 历史资料追溯

当前开发从[文档入口](../README.md)开始；当前工作状态由[具体任务](../tasks/)维护。

## 已从当前版本退役的材料

本轮整理前的完整仓库固定在
[`3db6fac2351d3fe4d3824aeca5aa0cbdb562693a`](https://github.com/zewbby/SpeechFUL/tree/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a)。
在任意本地完整克隆中可用 `git show 3db6fac2351d3fe4d3824aeca5aa0cbdb562693a:<原路径>` 读取；不要在当前运行目录恢复旧部署或执行旧清理脚本。

- [旧术语 CONTEXT](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/CONTEXT.md)：当前定义已集中到根 `GLOSSARY.md`。
- [HANDOFF](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/HANDOFF.md)、[progress](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/progress.md)：旧多轮进度，不再作为新会话指令。
- [模块交接](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/module-acceptance-handoff.md)、[验收台账](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/module-acceptance-ledger.json)：旧提交与旧设备的验收边界，不是当前通过清单。
- 同一提交中的 `tools/check-identity-handoff.py`、`check-privacy-closure-handoff.py`、`check-non-ios-closure.py`、两个 `check-android-b5*-evidence.py` 及配套收集／写台账脚本只校验或维护上述旧快照，随之退役。它们没有替代当前 Go、Flutter、SQL 或设备行为测试。
- [F1 页面计划](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/community-flutter-pages-plan.md)、[出图清单](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/docs/design/ui-reviews/f1-completion-audit.md)以及 F2 的设计、接口／依赖锁、验证记录、专用检查和本地 SQL 快照：只描述早期文字切片，不再约束后续功能或当前存储。实际行为继续由现行源码、正式迁移、契约和行为测试验证。
- 导航设计明确废弃的 v7／v8 六张分类列表图、v8 评审以及取消的“删除后独立页面”图：原始资产仍在上述固定提交中；当前文档引用改为历史链接。保留的 v9 列表图同时供“发布返回”流程引用，不再复制同一图片。

旧报告和 JSON 中记录的路径、命令、SHA 与 PASS／FAIL 保留原意，应结合其被测提交解读。退役清单与依赖核对见[整理任务](../tasks/REPOSITORY-CLEANUP-01.md)，不再维护新的逐文件摘要台账。

## 仍保留的历史来源

- [早期讨论](../discussions/README.md)：产品决定来源，不是当前状态。
- [旧分支记录](../branches/README.md)：对应任务与交付时点，不代表现在仍未合并。
- [文字帖原始归档](community-text-posting-20261008/README.md)：有独立原始字节与契约校验，保持其格式。
- `docs/design` 中的日期化报告、评审和截图：按各自版本和证明范围阅读；当前规则入口见[设计索引](../design/README.md)。
