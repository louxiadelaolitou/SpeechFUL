# PUBLISH-CATEGORY-01 — 分离浏览分类与发布分类

- 模式：常规；负责人／状态维护人：当前 Session
- 分支／Worktree／PR：`codex/fix/independent-publish-category`；`E:/Hnuhole-env/source/SpeechFUL-independent-publish-category`；[PR #136](https://github.com/zewbby/SpeechFUL/pull/136)。基线为已 fetch 的 `origin/main@67cd2452347ad491424193f7ebd425597da44007`。
- 授权依据与边界：用户确认需求后指示“是的，拉分支开干”；覆盖本任务开发、验证及必要本地提交。Push、PR、合并和发布未获授权。
- 后续授权：2026-11-11 用户要求核对“排查注册待核对与账号找回 (2)”后“推送、合并，远端CI”；授权本任务 Push、PR、适用 CI 通过后合并。本轮不再制作／安装／分发新包，不部署，统一验收和新版本安装包留待用户后续安排。
- 需求确认：用户确认“浏览分类和发布分类彼此独立”；推荐或任意分类只影响浏览，点击加号均独立选发布分类，不继承浏览分类、不跳过选择。选择发布分类不应反向切换浏览范围。无未决取舍。
- 更新日期：2026-11-11
- 状态：实现 已完成；验证 PASS（本地，远端结果见 PR 检查，真机 NOT_RUN）；合并 以 PR #136 实时状态及合并 SHA 为准；发布 未发布

## 目标与范围

首页及共用加号入口的新帖流程独立选择分类；统一按钮的可见提示与无障碍语义。保留既有草稿恢复、发布任务重试的分类绑定，以及搜索范围规则。旧 `ChannelPostsScreen` 只在历史组件测试中使用，当前生产入口为 `FeedScreen`。

## 接口与依赖

复用授权分类目录与分类选择面板。移除 `FeedScreen.composeChannel` 默认值入口；浏览状态继续归信息流所有，选出的发布分类只用于新草稿。不更改服务端接口、数据库及认证 Gate。

## 验收标准

- 推荐和所有浏览分类点击加号都显示“选择帖子分类”；没有推荐选项，没有预选分类。
- 选择与浏览不同的分类后，编辑页、确认页和发布操作使用本次选择；原浏览分类、选中态、已加载分页与阅读位置不因选择而改变。
- 取消分类选择不创建草稿；编辑返回保留原浏览范围；再次新建仍需独立选择，不能继承上次发布分类。
- 同分类发布与推荐流沿用原刷新逻辑；向其他分类发布不切换浏览范围。
- 分类选择期间会话失效时隐藏授权目录并阻止创建草稿；共用加号仍保留当前身份。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现调查 | 基线 `67cd245` | 现状为浏览分类直接写入新草稿；推荐进入时预选首分类，且选发布分类会调用 `loadTimeline` 切浏览范围。与本次已确认需求不同。 |
| 实现 | 基线 `67cd2452347ad491424193f7ebd425597da44007`＋本任务提交差异 | `feed_screen.dart` 每次新建均弹出无预选的发布分类面板，选中值只交给新草稿；删除反向 `loadTimeline` 和 `composeChannel` 默认入口；统一加号提示。更新主入口、关联调用测试、术语表，并将新回归加入 Feed UI 产品 CI。 |
| 依赖与格式 | Windows；Flutter 3.47.5／Dart 3.13.4；同基线＋本任务差异 | PASS：`flutter pub get --offline --enforce-lockfile`（`PUB_HOSTED_URL=https://pub.dev`），锁文件无差异；显式 Dart 文件 `dart format` 和 `git diff --check` 通过。沙箱首次 SQLite 官方资产下载受阻，正常工具环境下载后测试通过；未修改全局 Git 配置。 |
| 静态分析 | 同一最终源码及回归；`flutter analyze --no-pub` | PASS：无问题。[analyze.log](E:/Hnuhole-env/publish-category-validation-20261111/analyze.log)。 |
| 分类／身份交互回归 | 同一最终源码及回归；`flutter test --no-pub test/publish_category_test.dart test/active_identity_test.dart --reporter expanded` | PASS：15 项。覆盖推荐＋七分类均独立选择、无预选、取消不建草稿、选后编辑／保存／再新建、跨分类编辑→确认→发布返回保留模式／分页／游标／滚动、同分类和推荐顶部自动刷新、选分类期间注销、共用加号当前身份。[publish-category-tests.log](E:/Hnuhole-env/publish-category-validation-20261111/publish-category-tests.log)。使用内存保存／发布端口，仅证明生产界面和导航行为，不证明真实 SQL／HTTP／发布。 |
| 关联扩大回归 | 同基线＋最终产品与已有测试差异；`flutter test --no-pub test/feed_ui_test.dart test/feed_scroll_refresh_test.dart test/feed_read_error_scope_test.dart test/post_screens_test.dart test/post_controller_test.dart test/login_home_navigation_test.dart test/app_navigation_test.dart test/personal_route_account_test.dart test/mobile_flow_theme_integration_test.dart test/post_search_test.dart test/talkback_semantics_test.dart --reporter expanded` | PASS：11 文件、205 项；信息流、刷新、草稿／发布控制器、账号窗口、搜索、导航、主题与无障碍回归。[regression.log](E:/Hnuhole-env/publish-category-validation-20261111/regression.log)。之后仅补充新测试文件与任务证据，产品及这 11 文件源码未继续变更。 |
| Android 目标平台构建 | 同基线＋最终产品差异；`flutter build apk --debug --no-pub --target-platform android-arm64` | PASS：assembleDebug 83.8 秒。[android-build.log](E:/Hnuhole-env/publish-category-validation-20261111/android-build.log)；本地 APK SHA-256 `8f613e36fbc9a59c69294645e14e3dcf6d0cf4a2b690f9de0203ce966d0ae8ae`。未安装设备；不代表真机／生产验收。 |
| 差异与协作检查 | `git diff --cached --check`；`python tools/check-collaboration.py --staged`；暂存路径与未跟踪文件核对 | PASS；仅本任务 10 个文件，锁文件及生成物未进入暂存；协作检查不代替产品验证。 |
| 产品 CI／真机 | 本任务已推送，PR #136；未安装或操作手机 | 远端 CI 运行及最终结果绑定 [PR #136 检查](https://github.com/zewbby/SpeechFUL/pull/136/checks)；Feed UI workflow 已包含本任务回归，须当前 head 的适用检查全部通过后合并。真机 NOT_RUN。 |

补充自动刷新用例初次将正常的偏好读取误判为额外信息流请求；按验收范围仅断言刷新分类后，最终 15 项通过。初次日志保留为本机证据目录中的 `publish-category-tests-initial.log`，无对应产品修补。

远端首轮 head `131b986`：Feed UI（含真实 HTTPS／PostgreSQL）、认证、iOS、身份、私信、评论核心、图片、Android 构建／恢复演练及协作检查通过。Shared rate limit 与 Comment snapshot Redis 在安装 Redis 依赖时连续遭遇 `azure.archive.ubuntu.com` DNS 失败，未执行业务 SQL；仅重跑失败任务仍复现。已复用关联 PR #135 的 `f5875e1` 中这两个 workflow 的精确镜像修复：仍用签名校验的 Ubuntu 仓库，替换失效镜像地址，并设置有限重试及超时。未引入其认证源码或改写其任务记录；本任务新 head 的全部适用 CI 重新验收，不能用旧结果代替。

第二轮 head `9f69faa` 上述九组检查再次通过，但官方 Ubuntu 域名同样解析受阻。继续精确复用 #135 的 `37765c1` 中 `tools/ci/ubuntu-apt-source.sh` 及限流、快照、治理三个 workflow：仅在临时 GitHub Linux runner 运行配置写入；先验证 noble／noble-security 的 Ubuntu 元数据签名及发行标识，再配置可达官方签名镜像；保留 APT 信任根、包校验及有效期检查。脚本 `bash -n` 通过，全部适用 CI 以本次最终 head 重新核对。

关联会话随后核对发现新增触发的 Gate 与快照进程验收还有两个 Redis 安装入口未接入回退；精确复用其 `2ddf037` 中 Gate 和快照 workflow 的补全。分类产品源码仍为 `dafa188`，认证接口／数据库／正式环境保持本任务范围不变。最终适用 CI 包括新增触发的 Authorization Gate，不将前轮安装受阻或取消计为通过。

## 阻碍与下一步

本地实现提交 `dafa1881639929f080daf3378a9c76f893d06433`，220 项测试、静态分析及 Android 调试构建完成。已核对关联会话及 PR #135：已整合 #134，改动文件与本任务不重叠；新增注册结果接口尚未正式部署，统一验收需包含后端部署安排。两个任务均遵循本轮只合并、不部署、不打新包的约定。正在按授权推送并等待远端 CI，合并前核对最新 main；真机留待统一验收。
