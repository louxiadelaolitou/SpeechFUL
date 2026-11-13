# RELEASE-V1.1.2-FRP-20261113 — v1.1.2 公网安装包

- 模式：常规；负责人／状态维护人：当前 Session；无执行子 Session。
- 分支／Worktree：`codex/build/v1-1-2-frp`；`E:/Hnuhole-env/source/SpeechFUL-v1.1.2-frp`。
- 实际基线：v1.1.1 源码 `c3307e288f79c525af2ddbb5214dbd0b7a0c3e52`；本次远端 main `16c3e3ea330f68d5891366b53346d6f9287f6444`。
- 授权依据与边界：用户要求将 v1.1.1 打包之后已完成的改动及 FRP 公网入口统一打包为 v1.1.2；承接本聊天已说明的整合、后端准备、构建及验证顺序。保留既有数据、认证及 Safety Gate；GitHub Release 发布独立报告。
- 需求确认：包含 main 已合入的 #140／#141／#142，以及已完成的本地上传诊断与 FRP 工具；保留 publictrial 安装身份、原签名，Release／arm64。没有新增产品取舍；尚未实施的 R2 迁移、原图上传期限及暂停状态改进不冒充本次已完成内容。
- 更新日期：2026-11-13。
- 状态：实现 进行中；验证 NOT_RUN；合并 已有 PR 已合并、本次整合未合并；发布 未发布。

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
| 本地整合 | 本分支 | 进行中；上传 API 测试尾部同位置新增用例冲突，保留主线拉黑测试与上传诊断测试。 |
| 现有服务 | Windows／WSL；现有状态工具 | PASS：两个服务 active，本机 C/V readiness 200；不代证版本或公网业务。 |
| 现有数据库 | 归属 PUBLIC-TRIAL-ACCESS-01 的只读 SQL | PASS：实际 C38/V8，Gate C OPEN:1、V OPEN:3；需应用 C0039，保留既有 V8。 |
| 设备 | 现有 ADB 5037 只读设备清单 | NOT_RUN：未连接设备。 |

## 阻碍与下一步

完成整合与发布输入固定，运行分析／测试，核对后端升级需要，生成并验签 APK；设备连接后再完成真机验收。
