# 近期工作

近期工作集中在本文维护，README 仅保留入口。各项实现、验证、合并与发布状态以链接中的任务记录及 PR 为准；新增工作放在列表顶部，详细进度继续写入对应任务记录。

## 工作索引

- [全仓过期文件与历史分支退役](tasks/STALE-FILE-CLEANUP-20261110.md)：[PR #130](https://github.com/zewbby/SpeechFUL/pull/130) 已合并，退役 126 个文件和 64 条完成的远端分支，保留正式迁移与功能测试，修复 F4 镜像依赖；未部署。

- [Android 公网发布与 v1.0.0](tasks/ANDROID-BETA-LAUNCH-20261109.md)：后端升级、同包覆盖升级、管理员登录与测试内容清理已验证；v1.0.0 构建及 GitHub Release 状态见任务记录。

- [个人电脑公网试用接入](tasks/PUBLIC-TRIAL-ACCESS-01.md)：两个 HTTPS 入口、真实校邮收信与 Windows 登录后自动恢复已在独立试用环境验证；运行版本、重启复验和剩余缺口见唯一记录，正式发布未执行。

- [Android首版完整业务验收](tasks/ANDROID-V1-ACCEPTANCE-01.md)：PR #113 已合并；后续修复、旧 FAIL 与未验缺口按该任务的版本分别阅读。公网 v1.0.0 发布以 Android 上线任务为准，不将早期验收快照当作当前发布状态。

原有条目迁自 README 的 Related Work；相关任务扩展时更新对应入口，其他条目保留迁移时的描述与顺序。

- [帖子自定义标签](tasks/CUSTOM-TAGS-01.md)：PR #109 已合并，支持添加、复用、编辑、标签结果和关键词搜索；每帖最多三个、每个最多十字。旧版本日常部署与装机回执已归集，按任务的具体版本解释。

- [帖子分享与原帖链接](tasks/POST-SHARE-01.md)：后端与 App 接线已进入 main；系统分享、冷／热启动及认证后恢复原帖的验证范围和平台缺口见对应任务。

- [开发环境旧启动入口修复](https://github.com/zewbby/SpeechFUL/blob/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/branches/fix-development-runtime-resume.md)：三个原入口已统一版本绑定恢复器，C28/V7与三Redis冷启、重复执行验证通过，PR状态见任务。

- [隔夜关机会话恢复](https://github.com/zewbby/SpeechFUL/blob/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/branches/docs-session-overnight-acceptance.md)：单台 vivo 2702 隔夜恢复限定验证通过；C28/V7 与 Redis 恢复说明已整理，文档合并状态见任务PR。

- [欢迎图案与启动衔接](https://github.com/zewbby/SpeechFUL/blob/50baa3c2768b69188f2d6562cdb8b6c51e9fa674/docs/branches/codex-feat-welcome-mascot-logo.md)：[PR #88](https://github.com/zewbby/SpeechFUL/pull/88) 实现浅黑深白轮廓、黑底白标桌面图标及欢迎控件入场；实现与验证见任务记录，合并状态见 PR。

- [PR #84](https://github.com/zewbby/HnuHole/pull/84)：主线默认Redis新建与双读，标准启动配置接通，缺快照配置明确报错。
- [PR #83](https://github.com/zewbby/HnuHole/pull/83)：同步Redis快照合并和开发启用回执，保留历史验收边界。
- [PR #80](https://github.com/zewbby/HnuHole/pull/80)：评论Redis分页快照、局部恢复与Publish优化已合入main；同机开发已启用，生产未发布。
- [PR #82](https://github.com/zewbby/HnuHole/pull/82)：补充通知主线构建、同机开发后端与vivo冒烟交付回执，完整体验待用户试用。
- [PR #81](https://github.com/zewbby/HnuHole/pull/81)：四分类通知、跨模块身份同步与统一发帖导航已合入主线，主线包和同机开发服务已更新。
