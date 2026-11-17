# BLACKLIST-REDESIGN-20261116 — 黑名单页面重写

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：原分支 `codex/feat/blacklist-redesign`；独立 Worktree `blacklist-redesign/SpeechFUL.git`；用户要求与信件项目统一交付，代码已合入 `codex/feat/letter-to-students` 的统一 PR [#8](https://github.com/zewbby/SpeechFUL-Develop/pull/8)，原 PR #9 作为重复项关闭。
- 实际基线：最初为 `develop/main@1d829b2623a6c32830ee1b74fae6c99a436ad3b3`；交付前于 2026-11-17 合并最新 `develop/main@fed946b0d67dceb8ef9e27cd57310cf9ef9e7b40`。`origin` 是旧仓库，当前开发与最近黑名单 PR 均在 `zewbby/SpeechFUL-Develop`。
- 授权依据与边界：用户 2026-11-16 要求“新建分支，黑名单页面重写并美观化，然后PR”；覆盖实现、验证、本地提交、推送任务分支及 PR，不含合并、发布、装机或正式数据操作。原工作区“一封信”改动保留。
- 需求确认：本轮要求重写和美观化；沿用 AUTHOR-BLOCK-UNIFICATION-01 已确认的单一账号级拉黑、头像／昵称／描边“解除”、无返回箭头、无分区说明、空态“暂无记录”、简化确认面板。无新增业务取舍。2026-11-17 用户确认 [信件与黑名单视觉参考](../../design/ui-drafts/letter-blacklist-reference-v1.html) 并明确按稿改动，随后交付 PR；本轮继续保持既有产品语义。`grill-with-docs` 及其依赖在仓库与本机技能目录不可用。
- 更新日期：2026-11-17。
- 状态：实现已完成（视觉改版 `710bc73`）；本机聚焦验证 PASS（Flutter analyze／74 项聚焦回归；bundle BLOCKED）；统一 PR CI 以 #8 最新 head 为准；合并未合并；发布未发布。

## 目标与范围

重写 Flutter 黑名单页面的布局、加载、空态、错误和解除操作反馈；保持现有入口、接口、账号级拉黑含义与匿名边界。兼容浅深主题、长昵称、小屏和大字。修复旧页面在账号窗口变化后保留旧列表、确认期间重复打开面板的风险。

## 接口与依赖

沿用 `PostController.blockedAuthors`、`unblockAuthor`、`reloadVisibleFeeds` 与 `authorityVersion`。不修改服务端、迁移或 API。独立工作区不携带原工作区未提交内容。

## 验收标准

- 设置进入黑名单，显示当前账号的头像、昵称和“解除”；无返回箭头或身份关联信息，系统返回正常。
- 首次加载显示占位，读取失败可重试；成功空名单才显示“暂无记录”。
- 确认面板仅显示头像、昵称、“解除拉黑”和“取消”；取消不写入，同一时刻只打开一个面板。
- 解除期间按钮显示进度并防止重复操作；成功移除相应记录并刷新原内容，失败保留记录可重试。刷新失败不得恢复已解除的行或伪造空态。
- 会话／账号窗口变化后旧记录和动作失效，不能用新账号执行旧记录的解除。
- 浅深主题、小屏大字、长昵称及横屏布局无溢出；相关产品测试与当前 PR CI 如实记录。

## 设计卡

- 重点：在安静的列表中清楚识别对象和“解除”动作；不新增统计、分类、帮助说明或装饰卡片。
- 字体：沿用 Roboto／HhNotoSansSC；标题 22／700、昵称 16／500，昵称最多两行；按钮 14／500，随系统字号缩放。
- 色彩：复用 HhTelegramPalette 的 surface、background、ink、secondary、line、error；强调仅用于可执行动作。不是全局主题改版。
- 布局：居左标题、24dp 页边距、48dp 头像／触控、88dp 起的列表行；内容最大宽度 640dp，列表底部尊重安全区。
- 技能：hnuhole-ui-craft、frontend-design、ui-ux-pro-max 和 Impeccable Operate 参考；本地 Flutter 检索匹配异步状态、Semantics。直接生成并审查真实 Flutter 渲染截图，图稿不能代证设备验收。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 调查 | `1d829b2`；Windows | PASS：核对根目录、远端、分支和已有改动；建立独立任务分支；已阅读页面、设置入口、控制器、既有测试及产品规则。 |
| 实现 | 基线 `1d829b2`＋工作树差异 | 完成独立 `blacklist_screen.dart`；列表、加载、空态、失败重试、解除进度、确认面板和会话窗口失效处理已重写；接入产品 CI 与截图归档。 |
| 视觉 | Flutter 3.47.5；Windows；真实字体 | PASS：已查看 320×740／1.8 倍字号浅深主题、402×874 空态与解除面板 PNG；中文、按钮、长昵称截断及分隔线正常。路径为 `apps/mobile/build/blacklist-render/`，CI 产物名 `feed-ui-host-renders`。 |
| 视觉改版 | `710bc7357b5635032cce518701cbbf68d86a1e44`；Flutter 3.47.5；Windows；真实字体 | PASS：用户确认稿已落地为淡色头像底色、圆形用户失效空态、拖拽提示和 16px 顶角确认面板；主操作改为已确认深灰蓝。重新生成并查看 `list-402`、`empty-402`、`confirm-402`、320×740／1.8 倍浅深主题截图，长昵称和按钮无溢出。 |
| 视觉改版回归 | 同上 | PASS：`flutter analyze --no-pub` 无问题；信件、黑名单、Feed、设置、本地化和读屏聚焦测试共 74 项通过；协作规则 19 项通过。 |
| 最新 main 同步 | 合并提交 `b6a1f5a`；`develop/main@fed946b` | PASS：自动合并无冲突；合入后重新执行 `flutter analyze --no-pub` 无问题，同一组 74 项 Flutter 聚焦测试及 19 项协作回归通过。 |
| 验证 | 工作树差异；Flutter 3.47.5 | PASS：`flutter analyze --no-pub` 无问题；`flutter test --no-pub test/feed_ui_test.dart test/profile_settings_flow_test.dart test/blacklist_screen_test.dart test/app_localizations_test.dart test/talkback_semantics_test.dart` 共 61 项通过。新增测试覆盖加载、取消、解除失败重试、会话变化、小屏大字浅深渲染、空态和确认面板。 |
| 验证 | 同工作树；Windows | BLOCKED：`flutter build bundle --debug --no-pub` 因本机未安装 Android SDK 未完成；由产品 CI 覆盖构建。 |
| 统一交付 | 合并提交 `81971208d0eb7599a9d9851cd2c3ec098e038316`＋任务记录更新；Windows／Flutter 3.47.5 | PASS：信件、黑名单及 Feed UI 聚焦测试共 60 项通过；统一 PR #8 的最终 CI 以实时状态为准。 |

## 阻碍与下一步

统一 PR #8 已承接本任务；最终 head CI 以 PR 实时状态为准，合并和发布保持独立。真机读屏、触控与装机仍 NOT_RUN。
