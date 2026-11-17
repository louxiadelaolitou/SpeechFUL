# RELEASE-V1.2.0-20261117 — 汇总 v1.1.2 后有效 PR 并发布 Android v1.2.0

- 模式：常规；负责人／状态维护人：当前 Session；无子 Session。
- 分支／Worktree：`codex/build/release-v1-2-0-20261117`；`D:/SpeechFUL/.worktrees/release-v1-2-0-20261117`。
- 实际基线：`origin/main@22071c8d06aa5c4e564139c59672a3767609a92e`；上一发布 `v1.1.2@e43f7a436d49ce6a67ad30d69f1eeeda9288a61b`，APK SHA256 `c0d07c8aa02c897ccadeaf810664910a6c96837473b198fd17832b46980a0ffb`。
- 授权依据与边界：用户要求汇总 v1.1.2 后所有有效 PR、生成 v1.2.0 安装包并发布 GitHub Release。明确确认包含 #8 和 #5，整合验证后一起发布。覆盖独立发布分支整合、必要修复、构建验证、发布所需推送和标签／Release；不修改原 main、旧 Release、旧标签或真实业务数据，不自动部署配套后端。
- 需求确认：按 `grill-with-docs` 及其 grilling/domain-modeling 指南核实事实后集中确认。用户选择“包含 #8、#5，整合验证后一起发布（建议）”。#9 为 #8 重复关闭项，不重复叠加；其余有效产品 PR #1/#2/#3/#4/#6/#7/#10/#11/#12/#13 已在 main。沿用原 publictrial 包名、原签名、Release/arm64 和现有双 FRP origin，版本 1.2.0／2026111701。无未决产品取舍。
- 更新日期：2026-11-17。
- 状态：实现 进行中；验证 NOT_RUN；合并 发布分支整合中、未合入 main；发布 未发布。

## 目标与范围

从最新主线整合 #8 `d3dcf7cc49651e788175832aef394bf9c13c307e` 和 #5 所在功能分支，构建可覆盖 v1.1.2 的 Android APK，并将源码、版本、签名与摘要绑定到新 Release。既有本地 `v1.2.0` 指向旧 `f5e77b3`，远端不存在该标签；本任务保留本地旧标签，使用精确发布 SHA 创建远端新标签。

## 接口与依赖

- #8 新增首次阅读信件及新版黑名单；#5 通知读取优化依赖新增迁移。
- main 已有 `0049_post_search_documents.sql`；#5 原 `0049_notification_read_indexes.sql` 顺延为 `0050`，保留 main 已有迁移文件和搜索 schema 摘要；迁移账本断言与 workflow 路径同步。
- 发布沿用既有试用签名身份；证书 SHA256 `1538fb309499d651d7d12d7711a646c4a80fde4b1397857a9e39ce672b3dc9ae`。这仍是历史试用签名，不宣称生产签名治理完成。

## 验收标准

- #8、#5 与最新主线同时存在，迁移编号唯一，fresh/upgrade 和受影响产品 CI 通过。
- Flutter analyze、相关完整回归与 Android Release 构建通过；APK 包名、版本、证书、编译 origin 和 SHA256 正确。
- 新 v1.2.0 Release 的 tag 精确指向构建源码，下载资产与本地产物摘要一致，旧 v1.1.2 保留。
- SQL、手机、后端部署与发布分别记录，未执行项如实标记。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 事实调查 | GitHub REST 及本地仓库 | PASS：远端仅 v1.1.2 Release；#8 open，#5 已合入功能分支但未进入 main。原始快照位于 `E:/Hnuhole-env/release-v1.2.0-20261117/`。 |
| 整合 | 基线＋#8/#5 | 进行中。#8 自动合并成功；通知迁移与搜索迁移的编号冲突修复中。 |
| 验证／发布 | 待绑定最终 SHA | NOT_RUN。 |

## 阻碍与下一步

完成整合与迁移兼容验证，构建签名 APK，运行适用 CI 后发布；不以安装包构建证明后端新优化已部署。
