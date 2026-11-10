# 产品与技术设计索引

全仓库从[开发文档](../README.md)进入；当前术语统一见 [GLOSSARY](../../GLOSSARY.md)。本页索引规则与设计来源，不记录任务进度或累计验收结论。

## 阅读顺序与新旧边界

先读对应任务中已经确认的需求变更，再读模块设计和协议细节。较新的明确变更替代同一范围的旧规则；旧报告的 PASS、旧图的“已确认”不自动成为当前版本验收。

| 范围 | 当前确认来源 | 被替代的描述 |
| --- | --- | --- |
| 登录、找回密码与账号安全 | [校邮重置与暂停恢复凭据](../tasks/AUTH-EMAIL-RESET-01.md)、[账号与访问](modules/accounts.md) | 校邮仅用于注册、必须确认恢复码、用户可绑定或使用 Passkey |
| 分类、导航与设置 | [个人页与设置](../tasks/PROFILE-SETTINGS-01.md)、[最新排序](../tasks/POST-LATEST-SORT-01.md) | 旧入口树、旧设置手势和旧界面状态不能直接作为当前验收要求 |
| 通知 | [私信恢复与通知](../tasks/DM-RESUME-NOTIFICATIONS-01.md) | 早期系统消息与后续通知按对应版本区分 |
| 搜索、榜单与标签 | [搜索](../tasks/POST-SEARCH-01.md)、[全站热榜](../tasks/GLOBAL-HOTLIST-01.md)、[标签](../tasks/CUSTOM-TAGS-01.md) | 早期每分类帖子榜、具体权重尚未确定等不能覆盖后续确认 |
| 帖子编辑与分享 | [编辑](../tasks/POST-EDIT-01.md)、[分享](../tasks/POST-SHARE-01.md) | 旧静态正文、尚未接线的图稿不代表当前实现 |
| 发布与平台状态 | [Android 上线记录](../tasks/ANDROID-BETA-LAUNCH-20261109.md)、[iOS 任务](../tasks/IOS-V1-01.md) | 旧分支“未合并／未部署／未设备验证”只属于当时版本 |

## 模块与技术来源

以下模块文件保留设计来源和日期化增量，阅读时结合上表；不把其中旧阶段状态当成现状。

| 模块 | 设计来源 |
| --- | --- |
| 账号与访问 | [账号](modules/accounts.md)、[C/V 隐私架构](auth-privacy-architecture-decision.md)、[注册协议](auth-privacy-registration-protocol.md)、[威胁模型](auth-privacy-threat-model.md) |
| 身份 | [预设身份](modules/identity.md)、[头像与首次设置](../branches/identity-avatar-onboarding.md) |
| 导航、分类与标签 | [导航](modules/navigation.md)、[分类与标签](modules/tags.md) |
| 信息流与榜单 | [信息流接口](feed-backend-contract.md)、[榜单设计来源](modules/popularity.md) |
| 帖子发布与详情 | [新建帖子](modules/post-composer.md)、[详情](modules/post-detail.md)、[发布事务](community-text-posting-backend-design.md) |
| 评论与回复 | [评论规则](modules/comments.md)、[业务接口](../../packages/openapi/comment-business-component.yaml) |
| 消息与私信 | [消息](modules/messages.md)、[私信](modules/messaging.md)、[私信聊天契约](../../packages/openapi/direct-message-chat.yaml) |
| 个人内容与设置 | [设计来源](modules/personal.md) |
| 举报、审核与申诉 | [治理规则](modules/moderation.md)、[审核工作台](../../apps/admin/README.md) |
| 数据与工程基线 | [ADR](../adr/)、[正式迁移](../../services/api/migrations/)、[V 迁移](../../services/api/verifier-migrations/) |

## 图稿与历史

[UI 设计与复审流程](ui-quality-workflow.md)说明图稿的确认范围；`ui-drafts`、`ui-reviews` 和 [auth-flow-final](auth-flow-final/README.md) 保留各阶段设计。图稿不能代替实际页面或设备验收，旧恢复码／Passkey 演示不代表当前开放流程。

旧多轮交接、进度台账与原始设计索引通过[固定 Git 提交](../archive/README.md)追溯。历史图中未随仓库交付的 `UI产品图` 路径也应回到对应历史提交核实，不在当前入口提供失效链接。
