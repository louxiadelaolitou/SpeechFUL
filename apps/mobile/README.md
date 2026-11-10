# SpeechFUL Mobile

Flutter／Dart 手机 App。仓库内包名仍为 `hnuhole_mobile`；产品显示名为 SpeechFUL。

## 代码入口

- `lib/main.dart`：启动、认证与会话、各业务 Controller 和 API 的装配。
- `lib/src/navigation`：主标签页和菜单；`posts`：信息流、发布、详情、搜索与互动。
- `lib/src/auth`、`identity`、`comments`、`messaging`、`reports`、`system_notifications`：各业务界面与状态。
- `lib/src/storage`：本地帖子及媒体；认证秘密通过 [auth_vault](../../packages/auth_vault/README.md) 保存。
- `android`、`ios`：平台工程；复用插件在仓库 `packages`。

完整调用链见[代码导览](../../docs/development.md)。

## 配置与运行

配置固定且不同的 C/V HTTPS origin，不带路径、查询或响应指定的跳转地址：

```sh
flutter pub get --enforce-lockfile
flutter analyze --no-pub
flutter test --no-pub
flutter run --dart-define=AUTH_COMMUNITY_BASE_URL=https://community.example.org --dart-define=AUTH_VERIFIER_BASE_URL=https://verifier.example.org
```

默认地址为 `.invalid`，需要实际的 C/V 服务与系统信任的 TLS 证书。每组固定端点使用独立安全存储 scope；没有普通文件存储或证书校验绕过入口。后端初始化与 Windows／WSL 边界见 [API README](../../services/api/README.md)。

## 当前流程与确认来源

| 范围 | 入口与边界 |
| --- | --- |
| 登录与找回密码 | 用户名密码及校邮验证码登录；校邮重置后回登录页。恢复码和 Passkey 的用户流程暂停，保留实现与既有数据；见 [AUTH-EMAIL-RESET-01](../../docs/tasks/AUTH-EMAIL-RESET-01.md) |
| 设置与身份 | [PROFILE-SETTINGS-01](../../docs/tasks/PROFILE-SETTINGS-01.md)、[术语表](../../GLOSSARY.md) |
| 帖子、评论、私信与通知 | 已在启动代码中装配真实 API／运行时；入口与验证分层见[开发导览](../../docs/development.md)，发布范围见对应任务，不再沿用“评论／私信未开发”的旧阶段描述 |
| 图片与原生能力 | [图片查看器](../../packages/image_viewer/README.md)、[原生媒体](../../packages/native_media/pubspec.yaml)；平台与格式约束按实际实现和任务证据核对 |
| Android／iOS 验收 | [Android 上线记录](../../docs/tasks/ANDROID-BETA-LAUNCH-20261109.md)、[iOS 任务](../../docs/tasks/IOS-V1-01.md)；不能把 Android 或 host 通过外推为 iOS 通过 |

会话恢复以 C 为权威；成功响应的会话截止先安全持久化，再发布受保护数据。退出、换账号或新授权窗口使旧响应失效。401 与暂时不可用分开处理，不将超时视为已经退出或操作失败。

## 验证与历史

`test` 是单元与组件回归；`integration_test` 是设备／跨进程等集成入口。真实客户端→C/V HTTPS／PostgreSQL 联调需要对应隔离 runner，普通 `flutter test` 不会完成这些验证。

近期工作看 [RECENT_WORK](../../docs/RECENT_WORK.md)，具体范围与被测提交看任务记录。原 README 中的 2026-10 阶段接线、图片租约和恢复凭据交接说明可从[历史提交](https://github.com/zewbby/SpeechFUL/blob/3db6fac2351d3fe4d3824aeca5aa0cbdb562693a/apps/mobile/README.md)追溯；其“未合并”“待验”和设备环境均不代表当前机器状态。
