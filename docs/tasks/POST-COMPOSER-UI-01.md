# POST-COMPOSER-UI-01 — 发帖编辑页工具区、表情与帖子类型切换

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/feat/post-composer-ui`；`E:/code_collection/java_code/SpeechFUL-post-composer-ui`；[PR #22](https://github.com/zewbby/SpeechFUL-Develop/pull/22)。
- 授权依据与边界：2026-11-19 用户要求建立分支，优化发帖编辑页布局，支持表情与帖子类型切换，并最终提交 PR。授权覆盖本任务开发、必要本地提交、Push 与创建 PR；不包含合并、部署或正式发布。
- 需求确认：用户以“对”确认按 `图文 / 文字` 实现；类型只影响编辑界面和草稿，不进入发布 API；已有图片切换为文字时先提示移除，标题和正文保留；表情插入当前标题或正文光标位置。
- 更新日期：2026-11-19
- 状态：实现 已完成；验证 PASS（本地＋远端 25/25 检查）；合并 未合并；发布 未发布

## 目标与范围

发帖编辑页保留标题、正文、图片、下一步与草稿状态，新增底部固定工具区：`图文 / 文字` 分段切换、图片快捷入口、表情入口与表情面板。图文模式展示图片区；文字模式隐藏图片区。帖子类型写入本机加密草稿，不改变帖子协议、分类或已公开内容。

不包含：服务端帖子类型字段、公开帖子类型展示、内容推荐规则、图片上限调整、发布任务协议变更。

## 接口与依赖

- 复用 `PostComposerScreen`、`PostDraftEditor` 与现有加密草稿存储。
- 草稿数据新增 `composerType`，只保存 `IMAGE_TEXT` 或 `TEXT`；历史草稿缺少字段时兼容为图文。
- 发布载荷继续只包含标题、正文、标签、图片、身份与分类。

## 验收标准

- 编辑页底部工具区显示当前类型，`图文 / 文字` 切换后选中态与实际页面一致；图文显示图片区，文字隐藏图片区。
- 无图片时切换立即成功并保存草稿；已有图片时切换为文字先显示确认，取消不改变内容和类型，确认后移除图片但保留标题与正文。
- 类型在重启后从原加密草稿恢复；历史草稿按图文恢复。
- 表情面板与键盘互斥，支持常用、心情、校园分组；表情插入最近聚焦的标题或正文光标位置，并随草稿保存。
- 小屏、横屏和大字下固定工具区与表情面板不溢出；标题、正文和下一步仍可操作。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 基线 `213b8b0589ec919487a9516a1fa4f403ea5e1662`＋本任务差异 | 已完成类型模型、草稿兼容、编辑页工具区、表情面板、图片移除确认及多语言文案。 |
| 验证 | Windows Flutter 3.47.5 / Dart 3.13.4；`flutter analyze --no-pub` | PASS，0 问题。 |
| 验证 | 同上；`flutter test --no-pub test/post_screens_test.dart test/post_composer_media_test.dart test/app_localizations_test.dart test/community_localization_test.dart --reporter expanded` | PASS，58 项。覆盖类型切换、取消／确认移除图片、标题正文保留、光标插入、横屏大字、键盘收起过渡、草稿恢复与本地化。 |
| 关联回归 | 同上；`flutter test --no-pub test/post_screens_test.dart test/post_composer_media_test.dart test/post_composer_photos_test.dart test/post_controller_test.dart test/post_storage_test.dart test/post_models_test.dart test/post_edit_screen_test.dart test/active_identity_test.dart test/publish_category_test.dart test/mobile_flow_theme_integration_test.dart test/talkback_semantics_test.dart --reporter compact` | PASS，163 项。 |
| 全量 Flutter 尝试 | 同上；`flutter test --no-pub --reporter expanded` | 1247 PASS、4 SKIP、11 FAIL。失败均依赖本机缺失的 `openssl` 测试证书生成入口，未进入相关 TLS 断言；Linux 产品 CI 已另行通过。 |
| 实际渲染 | 402×874 逻辑尺寸、2 倍导出；`build/post-composer-render/` | PASS：图文、纯文字、表情三状态均已实际查看；证据仅证明本机 Flutter 布局，不代真机、键盘、触控与读屏验收。 |
| 远端 CI | PR #22 head `888560d06cfcd938f565d9c814240292167b0938` | PASS：25/25 个检查完成且无失败，`mergeable_state=clean`；`Feed UI` 与 Android 重新运行作业通过。 |

## 阻碍与下一步

- 真机安全区、系统键盘切换、触控尺寸、动态字号和读屏仍为 NOT_RUN；由当前 PR head 的产品 CI 与后续设备验收补充。
