# RELEASE-V1.1.2-FRP-20261113 — v1.1.2 公网安装包

- 模式：常规；负责人／状态维护人：当前 Session；无执行子 Session。
- 分支／Worktree：`codex/build/v1-1-2-frp`；`E:/Hnuhole-env/source/SpeechFUL-v1.1.2-frp`。
- 实际基线：v1.1.1 源码 `c3307e288f79c525af2ddbb5214dbd0b7a0c3e52`；本次远端 main `16c3e3ea330f68d5891366b53346d6f9287f6444`。
- 授权依据与边界：用户要求将 v1.1.1 打包之后已完成的改动及 FRP 公网入口统一打包为 v1.1.2；承接本聊天已说明的整合、后端准备、构建及验证顺序。保留既有数据、认证及 Safety Gate；GitHub Release 发布独立报告。
- 需求确认：包含 main 已合入的 #140／#141／#142，以及已完成的本地上传诊断与 FRP 工具；保留 publictrial 安装身份、原签名，Release／arm64。用户指出 Passkey 已先行移除；复核 `main.dart` 中恢复及管理控制器均为 `legacyRecoveryEnabled: false`，沿用已确认暂停策略，不恢复、不迁移认证域。底层 RP 校验仍保留，不把已暂停功能当成本次发布阻碍。尚未实施的 R2 迁移、原图上传期限及暂停状态改进不冒充本次已完成内容。
- 更新日期：2026-11-13。
- 状态：实现 本地整合和 APK 已完成；验证 本地 PASS、远程 CI 与真机待验；合并 已有 PR 已合并、本次整合未合并；发布 未发布。

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
| 本地整合 | `57cea75e0acbe3be8364a46b86b26f51b9a10437` | PASS；上传 API 测试尾部同位置新增用例冲突，保留主线拉黑测试与上传诊断测试；其他产品差异正常整合。 |
| 现有服务 | Windows／WSL；现有状态工具 | PASS：两个服务 active，本机 C/V readiness 200；不代证版本或公网业务。 |
| 现有数据库 | 归属 PUBLIC-TRIAL-ACCESS-01 的只读 SQL | PASS：实际 C38/V8，Gate C OPEN:1、V OPEN:3；需应用 C0039，保留既有 V8。 |
| Flutter 分析与回归 | `57cea75` 的构建副本；Windows Flutter 3.47.5；锁定离线依赖 | PASS：`flutter analyze --no-pub` 零问题；14 个受影响测试文件 194 项通过。[分析](E:/Hnuhole-env/release-v1.1.2-frp-20261113/flutter-analyze.log)、[回归](E:/Hnuhole-env/release-v1.1.2-frp-20261113/flutter-tests.log)。首轮受限环境无法下载 SQLite 测试二进制，后在允许构建网络的环境完成，不记为产品测试失败。 |
| Release APK | `57cea75`；正常 `lib/main.dart`；只作 publictrial 包名包装 | PASS：1.1.2／2026111301、Release／arm64；37,308,474 字节；SHA256 `ad7662eb343a7cd91feda221555388ea06c3cc359d92bb0ac79437c7278881cb`；签名 SHA256 `1538fb309499d651d7d12d7711a646c4a80fde4b1397857a9e39ce672b3dc9ae` 与 v1.1.1 相同。[构建回执](E:/Hnuhole-env/release-v1.1.2-frp-20261113/build-receipt.json)。 |
| 后端构建与静态检查 | 同源 `57cea75`；WSL Linux 文件系统、Go1.22.2 | PASS：C/V 候选编译；postmedia、posts、authprivacyhttp、authprivacyruntime 测试；`go vet ./...`。[Go 测试](E:/Hnuhole-env/release-v1.1.2-frp-20261113/go-tests.log)。 |
| 真实 SQL 与 Redis | `57cea75`；本轮 runner 新建的一次性集群，无外部 DSN | PASS：C39 fresh／upgrade、作者拉黑、并发死锁与隐藏图片等聚焦回归；独立 Redis AOF 重启回归。[SQL](E:/Hnuhole-env/release-v1.1.2-frp-20261113/postgres-tests.log)、[Redis](E:/Hnuhole-env/release-v1.1.2-frp-20261113/redis-test.log)。不代表现有库已升级。 |
| 运维工具 | 本轮整合内容 | PASS：启动清理、ACL 与上传代理脱敏边界共 9 项 Python 回归。 |
| 远程产品 CI | 本次整合分支 | BLOCKED：自动审批拒绝 push，理由为用户仅明确授权本轮打包，未明确授权本次 GitHub 推送；未改用其他方式绕过。待用户批准后推送并建 PR。 |
| 设备 | 现有 ADB 5037 只读设备清单 | NOT_RUN：未连接设备。 |

## 阻碍与下一步

APK 已生成；本次 source、构建输入及检查绑定 `57cea75`，后续仅任务证据文档更新不改变产品内容。既有 `MobileEnvironment.storageScope` 包含双 origin，新入口首次使用需重新登录，原本机环境材料不删除也不自动跨环境搬迁；服务器账号与帖子沿用原库。

需用户批准后执行本次 GitHub push／PR 产品 CI，以及现有公网试用后端升级（备份后 C38→39，V8 不变，替换已构建二进制、公开 origin 切至本次 FRP，内部 mTLS peer／Gate／暂停 Passkey 策略不变）。当前仅准备候选，不操作现有库或正式 Release。设备连接后再完成真机验收。
