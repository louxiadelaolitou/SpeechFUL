# search-hotlist-subtitle-20261111 — 删除搜索页热榜副标题

- 模式：常规
- 负责人／状态维护人：当前 Session
- 分支／Worktree：`codex/fix/remove-search-global-hotlist`；`E:/Hnuhole-env/source/SpeechFUL-remove-search-global-hotlist`；基线 `67cd2452347ad491424193f7ebd425597da44007`
- 授权依据与边界：用户要求“搜索帖子，大家都在看底下的全站热榜删去”，并授权简单测试后提交、推送、合并；不含部署。
- 需求确认：删除“大家都在看”下方的“全站热榜”副标题及专属间距；保留标题、榜单与搜索行为。无未决取舍。
- 更新日期：2026-11-11
- 状态：实现已完成；验证本地 PASS，CI 待验；合并未合并；发布未发布

## 目标与范围

搜索页空关键词状态只保留“大家都在看”标题与榜单，不再显示“全站热榜”副标题。仅删除一处 Text 和其专属 SizedBox，不修改请求、排序、历史或状态机制。

## 接口与依赖

无。

## 验收标准

- 进入搜索帖子页面，“大家都在看”正常显示，其下不再显示“全站热榜”。
- 搜索历史、榜单加载及帖子打开保持现有行为。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 基线 67cd245 + 本任务 post_search_screen.dart 的 8 行删除 | 已完成；差异仅删除副标题及专属间距 |
| 静态分析 | Windows / 现有 Flutter；`flutter analyze --no-pub lib/src/posts/post_search_screen.dart` | PASS，No issues found |
| 搜索页回归 | `flutter test --no-pub test/post_search_test.dart` | PASS，22 项通过；旧测试中两处要求副标题存在的断言已更新为标题存在、副标题不存在；首轮因此失败，修改后复跑通过 |
| 本地依赖 | `flutter pub get --offline` | 使用现有离线缓存解析；自动变化的锁文件已恢复，不随本任务提交；锁定依赖以 PR 产品 CI 为准 |
| 真机／数据库 | 本次仅删除展示文字 | SKIP，按用户要求不扩展验证 |

## 阻碍与下一步

待提交、推送，核对 PR 当前 head 的适用 CI 后合并。未授权也未执行部署。
