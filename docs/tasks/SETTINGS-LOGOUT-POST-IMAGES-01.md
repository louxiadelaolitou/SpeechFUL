# SETTINGS-LOGOUT-POST-IMAGES-01 — 设置退出登录与三张图片上限

- 模式：常规；当前 Session 维护状态，无执行子 Session。
- 分支／Worktree／PR：`codex/feat/settings-logout-post-image-limit-3`；本轮隔离工作区 `E:/Hnuhole-env/source/SpeechFUL-pr140-conflicts`；[PR #140](https://github.com/zewbby/SpeechFUL/pull/140)。
- 实际基线：原 PR head `8ef414860eb9eeb31999c8ba6a2d62c159f608e4`，同步 `main@972742c8fe317372aa990096aaaa2afc5f83c778`。
- 授权依据与边界：2026-11-13 用户要求“把 PR #140 的冲突处理一下，有什么冲突”；本轮完成冲突定位、解决、必要验证及推送回原 PR。未获合并或发布授权，不操作正式数据、设备或部署。
- 需求确认：沿用 PR 已确认的设置退出登录、移除账号与安全入口、新图片最多 3 张、历史 4–9 图读取兼容；本轮保留主线夜间主题、作者主页及账号级拉黑。两侧改动可同时成立，无新增产品取舍。
- 更新日期：2026-11-13。
- 状态：实现 已完成；验证 本地 PASS、当前 head CI 待验；合并 未合并；发布 未发布。

## 目标与范围

解除 #140 相对当前主线的合并冲突，保持既有退出与图片规则，以及主线新功能。

## 冲突与处理

两处文本冲突均来自 [PR #141](https://github.com/zewbby/SpeechFUL/pull/141) 的夜间模式适配：

- `post_composer_photos.dart`：#140 将图片计数从 `/9` 改为统一上限，主线同时调整计数文字颜色。保留 `PostContentRules.maxImages` 和动态主题色。
- `post_edit_screen.dart`：#140 将添加图片条件限制为 3 张，主线同时让图片格子接收 `BuildContext` 以读取主题。保留新调用签名和三张上限。

自动合并部分已审查；解决后的产品差异相对最新主线仍为原 PR 的 26 个文件、230 行新增及 106 行删除。不恢复旧主题常量、不覆盖作者主页与拉黑装配，不调整认证或安全 Gate。

## 验收与证据

本轮被测内容为 `8ef4148` 合入 `972742c` 后的暂存产品差异；最终合并提交承载相同内容。原 `8ef4148` 的历史 CI 见 PR 描述，不能代替本轮合并后 CI。

| 项目 | 环境／命令 | 结果与证据 |
| --- | --- | --- |
| 冲突与差异 | `git diff --cached --check`、相对主线差异复核 | PASS；仅两处需人工解决，未引入额外产品改动。 |
| 依赖 | Windows Flutter 3.47.5；`PUB_HOSTED_URL=https://pub.dev flutter pub get --offline --enforce-lockfile` | PASS；沿用锁文件来源，锁文件不变。初次沿环境默认镜像解析不满足锁文件，未采用其解析结果。 |
| Flutter 分析 | `flutter analyze --no-pub` | PASS；零问题。[日志](E:/Hnuhole-env/pr140-conflict-resolution-20261113/flutter-analyze.log)。 |
| 聚焦交互回归 | `flutter test --no-pub --concurrency=2 --reporter expanded`，图片选择／协议／编辑、主题、信息流、作者主页、设置退出、账号身份与登录导航共 14 个测试文件 | PASS，180 项；[日志](E:/Hnuhole-env/pr140-conflict-resolution-20261113/flutter-targeted.log)。 |
| bundle 构建 | `flutter build bundle --debug --no-pub` | PASS；[日志](E:/Hnuhole-env/pr140-conflict-resolution-20261113/flutter-bundle.log)。 |
| API 契约 | Python UTF-8 模式；`packages/openapi/check-specs.py`、`tools/check-community-text-contract.py`、`unittest discover -s tools/tests -p test_community_text_contract.py -v` | PASS；全部 OpenAPI、静态兼容检查及 10 项契约回归通过。 |
| 协作检查 | `python tools/check-collaboration.py --staged` | PASS；仅证明协作策略。 |
| 当前 head 远程产品 CI | 推送后由原 PR workflows 执行 | NOT_RUN；等待推送。真实 PostgreSQL 验证由隔离 SQL CI 覆盖，本地未宣称 SQL 通过。 |
| 真机／签名发布／生产验收 | 本轮不涉及 | NOT_RUN。 |

## 阻碍与下一步

本地验证已完成；提交并推送原 PR，然后核对 GitHub 合并状态与当前 head CI。README 无需变更。不同步历史进度／交接快照，不执行 PR 合并或部署。
