# RELEASE-V1.1.2-FRP-20261113 — v1.1.2 公网安装包

- 模式：常规；负责人／状态维护人：当前 Session；无执行子 Session。
- 分支／Worktree／PR：`codex/build/v1-1-2-frp`；`E:/Hnuhole-env/source/SpeechFUL-v1.1.2-frp`；[PR #144](https://github.com/zewbby/SpeechFUL/pull/144)（draft）。
- 实际基线：v1.1.1 源码 `c3307e288f79c525af2ddbb5214dbd0b7a0c3e52`；本次远端 main `16c3e3ea330f68d5891366b53346d6f9287f6444`。
- 授权依据与边界：用户要求将 v1.1.1 打包之后已完成的改动及 FRP 公网入口统一打包为 v1.1.2。2026-11-13 在本聊天的两项具体授权中明确回答“允许推送并创建 PR”“允许升级配套后端”；授权覆盖向 `zewbby/SpeechFUL` 推送本次分支／建 PR，以及按已准备方案备份并升级现有公网试用后端。保留既有数据、认证及 Safety Gate；不据此自动上传 GitHub Release。
- 需求确认：包含 main 已合入的 #140／#141／#142，以及已完成的本地上传诊断与 FRP 工具；保留 publictrial 安装身份、原签名，Release／arm64。用户指出 Passkey 已先行移除；复核 `main.dart` 中恢复及管理控制器均为 `legacyRecoveryEnabled: false`，沿用已确认暂停策略，不恢复、不迁移认证域。底层 RP 校验仍保留，不把已暂停功能当成本次发布阻碍。尚未实施的 R2 迁移、原图上传期限及暂停状态改进不冒充本次已完成内容。
- 更新日期：2026-11-13。
- 状态：实现 整合、最终 APK 及配套试用后端升级已完成；验证 本地与公网边界 PASS、远程 CI 以 PR 当前 head 为准、真机 NOT_RUN；合并 #140／#141／#142 已合并、#144 未合并；发布 APK 已生成、试用后端已启用、GitHub Release 未发布。

## 目标与范围

生成 v1.1.2、versionCode 2026111301 的可覆盖升级 APK。编译时使用社区 `https://www-api.h2454089d.nyat.app:53365` 与认证 `https://www-auth.h2454089d.nyat.app:48552`；原官网入口保留。先核对真实后端代码、C39/V8 schema、授权与公网链路；不把构建成功等同于完整认证或真机发图验收。

## 接口与依赖

- main #140：退出登录、新图片最多三张及旧帖子兼容；#141：帖子夜间主题；#142：账号级拉黑及 C0039 迁移。
- 本地 `codex/fix/post-upload-diagnostics@ddb9b77` 与 `codex/test/mainland-tunnel-pilot-20261112@94f1640`；在本次独立分支整合，不改原 Worktree。
- 现有 WSL 公网试用环境 `/home/hnuhole/public-trial-access-01/runtime`，后端与 FRP 各自运行；迁移前核对归属、备份与已有 schema。

## 验收标准

- APK 的源码、双入口、包名、版本、签名及 SHA256 可核对；与现有 publictrial 签名一致。
- 整合后分析、受影响交互／传输回归和 Release 构建通过；后端与真实数据库验证分别报告。
- 真机验收覆盖公网登录／冷启、浏览、发图及本次新增行为；设备不可用则保持待验，不伪报 PASS。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 基线核查 | GitHub 与本地 main `16c3e3e` | PASS；#140、#141、#142 已合并。 |
| 本地整合 | 最终 APK 产品源 `5a216114a2c454178404509a3fda81620f3ba0fc`；后端 `57cea75` | PASS；上传 API 测试尾部同位置新增冲突保留两侧；CI 首轮发现 Flutter foundation 进入纯 Dart 探针依赖链，`5a21611` 改用 Dart 自带日志输出及 Zone 捕获，保留脱敏与失败语义；后端产品代码与 main `16c3e3e` 一致。 |
| 现有服务 | Windows／WSL；现有状态工具 | PASS：两个服务 active，本机 C/V readiness 200；不代证版本或公网业务。 |
| 升级前数据库 | 归属 PUBLIC-TRIAL-ACCESS-01 的只读 SQL | PASS：原 C38/V8，Gate C OPEN:1、V OPEN:3；升级目标 C39/V8。 |
| Flutter 分析与回归 | 最终 `5a21611` 的构建副本；Windows Flutter 3.47.5；锁定离线依赖 | PASS：`flutter analyze --no-pub` 零问题；14 个受影响测试文件 194 项通过。[分析](E:/Hnuhole-env/release-v1.1.2-frp-20261113/flutter-analyze.log)、[回归](E:/Hnuhole-env/release-v1.1.2-frp-20261113/flutter-tests.log)。首轮受限环境无法下载 SQLite 测试二进制，后在允许构建网络的环境完成。 |
| 纯 Dart 探针 | 最终 `5a21611`；`dart compile exe` | PASS：feed_live、post_search_live、post_share_live、post_tags_live、feed_staging 五个探针均可编译；没有用 Flutter bundle 编译替代此项。 |
| Release APK | 最终 `5a21611`；正常 `lib/main.dart`；只作 publictrial 包名包装 | PASS：1.1.2／2026111301、Release／arm64；37,308,474 字节；SHA256 `915c30b714151fbdeef17f701359ae647672d125b8521e67b620d0d54b371e67`；签名 SHA256 `1538fb309499d651d7d12d7711a646c4a80fde4b1397857a9e39ce672b3dc9ae` 与 v1.1.1 相同。[回执](E:/Hnuhole-env/release-v1.1.2-frp-20261113/build-receipt.json)、[2031 个源码文件／编译双 origin 核对](E:/Hnuhole-env/release-v1.1.2-frp-20261113/final-source-verification.json)。首个未交付候选基于 57cea75，已由最终修复包替代。 |
| 后端构建与静态检查 | 同源 `57cea75`；WSL Linux 文件系统、Go1.22.2 | PASS：C/V 候选编译；postmedia、posts、authprivacyhttp、authprivacyruntime 测试；`go vet ./...`。[Go 测试](E:/Hnuhole-env/release-v1.1.2-frp-20261113/go-tests.log)。 |
| 真实 SQL 与 Redis | `57cea75`；本轮 runner 新建的一次性集群，无外部 DSN | PASS：C39 fresh／upgrade、作者拉黑、并发死锁与隐藏图片等聚焦回归；独立 Redis AOF 重启回归。[SQL](E:/Hnuhole-env/release-v1.1.2-frp-20261113/postgres-tests.log)、[Redis](E:/Hnuhole-env/release-v1.1.2-frp-20261113/redis-test.log)。不代表现有库已升级。 |
| 运维工具 | 本轮整合内容 | PASS：启动清理、ACL 与上传代理脱敏边界共 9 项 Python 回归。 |
| 现有试用后端升级 | `57cea75` 后端二进制；与最终 `5a21611` 的 Go／infra 产品树相同 | PASS：备份后 C38→39，V8 保持；C/V 及 authdev 二进制更新、公开 origin／监听端口／本机 TLS 与代理匹配；内部 mTLS peer、私有 CA、私钥与暂停 Passkey 策略不变。启动中发现公开端口约束及基础授权重置特性后，恢复有效配置、补齐四项已启用功能的规范最小授权，并先通过完整配置／证书／代理检查再切换。未绕过启动校验或 Gate。 |
| 数据与 Gate 保留 | 原库与私有备份；升级后只读比对 | PASS：现有账号、帖子及其他业务表摘要未变；C OPEN:1、V OPEN:3，RECOVER 次数不变。服务恢复后仅热榜派生快照与其分片引用发生正常刷新，单独记录，不声称全部表逐字节不变。[后端回执](E:/Hnuhole-env/release-v1.1.2-frp-20261113/backend-receipt.json)。备份位于 runtime/backup/v1.1.2-20261113T130642Z，未重置 schema、清数据或回退迁移。 |
| 公网 TLS 与未认证边界 | 新 FRP C/V；严格默认 TLS，无跳转 | PASS：C/V readiness 200，未带会话的频道与拉黑列表均按契约返回 401。[公网证据](E:/Hnuhole-env/release-v1.1.2-frp-20261113/public-smoke.json)。不代证真实登录或发图。 |
| 远程产品 CI | [PR #144 checks](https://github.com/zewbby/SpeechFUL/pull/144/checks) | 首轮 b6b2315 的 flutter-and-real-feed 因纯 Dart 引入 dart:ui 失败；5a21611 已修复，本地五探针 PASS，远程以当前 PR head 为准。首轮 push 曾因缺少明确授权被自动审批拒绝，用户随后已明确批准并完成 push／PR。 |
| 设备 | 现有 ADB 5037 只读设备清单 | NOT_RUN：未连接设备。 |

## 阻碍与下一步

最终 APK 已生成；移动端源码与构建输入绑定 `5a21611`，后端及一次性 SQL／Redis 测试绑定后端相同的 `57cea75`。后续仅任务证据文档更新不改变产品内容。既有 `MobileEnvironment.storageScope` 包含双 origin，新入口首次使用需重新登录，原本机环境材料不删除也不自动跨环境搬迁；服务器账号与帖子沿用原库。

用户批准的 push／PR 与配套试用后端升级已执行；核对当前 head 产品 CI，设备连接后再完成真机验收。未自动合并 #144，未上传 GitHub Release。新内部公网 listener 叶证书有效至 2026-11-20 21:15:20（Asia/Shanghai），既有私有 CA 至 2026-12-07 10:33:43；继续托管该开发／试用环境时需按其实际期限续期，不把 Sakura 外层证书自动化视为内层证书已续期。
