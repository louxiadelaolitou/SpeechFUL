# POST-THUMBNAIL-REDIS — 帖子缩略图 Redis 热缓存

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`codex/perf/post-thumbnail-redis`；`E:\speechful-post-thumbnail-redis`；[PR #34](https://github.com/zewbby/SpeechFUL-Develop/pull/34)（base `main`）
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户“阅读并完成任务”并指定《SpeechFUL 帖子缩略图 Redis 缓存实施方案（2026-11-23）》；2026-11-24 追加“提pr”“提完用/pr-review审一下，然后审完改了合并”，据此完成推送、PR #34、审查与修复并执行合并；未授权试用环境升级、生产部署或开启生产开关，本记录不产生方案外授权
- 需求确认：以方案正文为实现契约；默认关闭，PostgreSQL 仍为原图、衍生字节、版本、可见性与最终授权权威。未决产品取舍：Redis 私有副本最大逻辑残留时间、每实例资源预算、目标流量分配、生产开关阈值；若要求删除与 PostgreSQL 同事务抹除 Redis 字节，本方案不满足，生产开关保持关闭。迁移因基线已有 `0054` 顺延为 `0055`
- 更新日期：2026-11-25
- 状态：实现 已完成（本地提交 `176b1f0`）；审查 已完成并修复（`aa031c1`、`c3d4212`、`d74c2fa`、`8e3eb28`）；验证 已完成：PR #34 head `5781539` 的 26 项 checks 中 25 项 success、`actual-process-acceptance` 按设计 skipped（含 `rehearsal`、`go-postgres`、`gate-postgres`、`search-postgres-100k`、`flutter-and-android`）；合并 已完成：PR #34 squash 合并为 `main` 的 `803df8e`；发布 未发布（未升级试用环境、未部署、生产开关保持关闭）

## 目标与范围

在既有 PostgreSQL 私有缩略图复用路径前增加一个可丢弃、默认关闭的 Redis 热缓存，仅覆盖 `thumbnail-v1-640` 的受保护读取：

- 请求仍先读 PostgreSQL 元数据、执行原最终授权事务；Redis 命中值按 PostgreSQL 元数据校验长度、宽高、WebP 容器与 SHA-256，最终事务再次核对来源证明和衍生行身份。
- 独立 Redis 实例、`allkeys-lfu`、固定绝对截止时间；失败/超时/坏值回落有界 PostgreSQL 读取，再回到既有生成路径。
- 只有已存在的 PostgreSQL 衍生行且本次授权成功提交后才回填；`SET NX PXAT`，队列有界，截止时间锚定最终授权时间。
- 新增同事务 `AFTER DELETE` outbox、租约删除 worker、聚合指标、配置样例、授权脚本与设计说明。
- 不包含原图、头像、评论/私信图片、查看器保存、公开 URL、客户端离线缓存；不改客户端入口。

## 接口与依赖

- 新增 `internal/postthumbnailcache`：配置、键构造、`GETRANGE` 读取校验、有界回填队列、`UNLINK` 删除、熔断、指标。
- `authprivacy` 新增可注入 `thumbnailHotCache`；`WithPostThumbnailRedisCache` 启动时校验 outbox/触发器/函数摘要与运行时权限。
- 迁移 `0055_thumbnail_cache_outbox.sql`；授权 `infra/postgres/grant-post-images.sql`；示例 `infra/postthumbnail/redis.example.conf`。
- 运行时配置 `postImages.thumbnailRedis` 必须同时启用 `thumbnailReuse` 与 `preparedMediaReads`，命名空间绑定环境，地址须区别于共享准入与评论快照 Redis。
- CI `post-images.yml` 增加一次性 Redis 7.4 容器与命令/过期回归；隔离 PostgreSQL lab 仍须 Linux unix socket 环境。

## 验收标准

- 未配置 Redis 时行为与基线一致；配置非法、schema/触发器/权限不符、探测失败时启动失败关闭。
- 热路径：Redis 有效命中有界且不读 PostgreSQL payload、不进入 Python 生成；未命中/坏值/超时回落，输出与原路径一致。
- 回填：现场生成、授权冲突、提交失败均不回填；已存在衍生行且提交成功后只回填一次，绝对截止时间不晚于最终授权时间 + TTL。
- 删除：衍生行删除在同一 PostgreSQL 事务产生 outbox；worker 以租约认领、事务外删除精确键、可重试/接管；禁止 `KEYS`/全扫描；积压与最老年龄可见。
- 指标不含键、帖子/账号 ID 或令牌标签；日志仅聚合计数。
- 真实 PostgreSQL/Redis/Python/HTTPS 与 A/B 性能证据按方案矩阵记录；缺证据不得声称生产通过。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 提交 `176b1f0`，基线 `bc599b342da707169e55ddff90e8f2a54fc751cc` | 读路径、Redis 包、0055 迁移、worker、schema 校验、运行时装配、样例与文档；默认关闭 |
| 静态检查 | `176b1f0`；Go 1.22.2；`GOOS=linux GOARCH=amd64` | `go build -mod=readonly ./...` PASS；`go vet`（`postthumbnailcache`/`authprivacy`/`authprivacyruntime`）PASS；15 个改动 Go 文件去 CR 后 `gofmt -d` 无差异；`authprivacy`/`authprivacyruntime` 测试二进制交叉编译 PASS |
| 单元测试 | `176b1f0`；Windows amd64，Go 1.22.2 | `go test -count=1 ./internal/postthumbnailcache` PASS（配置/键/GETRANGE/NX PXAT/队列/熔断/删除/指标） |
| 真实 Redis | `176b1f0`；Redis 8.10.3（Windows 构建），`127.0.0.1:6391`，一次性无持久化实例 | `TestCacheAgainstRealRedis` PASS：写入、绝对 TTL、NX 不覆盖、同长损坏/超长拒绝、UNLINK |
| ACL 样例 | `176b1f0`；同上实例 | 受限用户 `~hnuhole:post-thumbnail:v1:test:* +ping +getrange +set +unlink`：`HELLO 3`/PING/GETRANGE/SET/UNLINK 可用；命名空间外键与未授权 `GET` 均返回 `NOPERM` |
| 真实 PostgreSQL 迁移冒烟 | `176b1f0`；Windows 便携 PostgreSQL 16.4，一次性本地集群 `127.0.0.1:55432`，最小 `c_posts.thumbnails` schema | PASS（仅迁移/触发器冒烟，非隔离 lab）：0055 Up 在事务内应用；函数 `prosrc` 去 CR 后 SHA-256 等于 `c709eae...`；触发器 `AFTER DELETE FOR EACH ROW`（mask 9）参数/条件/列/约束均空；PK `event_id` 与两个部分索引有效；运行时仅 SELECT/UPDATE/DELETE，无 INSERT/TRUNCATE/REFERENCES/TRIGGER/EXECUTE；以运行时角色删除衍生行仍由 definer 触发器生成且仅生成一条字段一致的 outbox 事件，直接 INSERT 被拒；更新后授权脚本 0055 块可重复执行；Go `checkThumbnailCacheSchema` 原始查询在 superuser 与 `SET ROLE hnuhole_c_runtime` 下均返回 true。冒烟中发现并修复 PK 断言误用 `NOT EXISTS` 及 `notnull` 别名语法问题 |
| 真实 PostgreSQL lab（全矩阵） | WSL2 `Ubuntu-2404`（Linux 6.6.87.2）；一次性 private unix-socket PostgreSQL 16.15；Pillow 12.3.0；真实 HTTPS C/V Gate fixture | NOT_RUN：WSL2 已可用，真机 fixture 已在隔离 private-socket PostgreSQL/Python/HTTPS 上跑通（见下行）；`run-post-images-isolated.sh` 完整矩阵尚未执行，不以 fixture 代替矩阵通过 |
| 真机同客户端复用 A/B | Xiaomi `24129PN74C`（Android 17／SDK 37，HyperOS V816）；代码 head `2e8bb2f`（APK 构建后工作树无产品差异）；profile APK SHA-256 `e1754fcbd70026152762d12b0640f1e8d34b735c7140ad8b513b0d156d152fa4`；WSL2 一次性 private-socket PostgreSQL 16.15＋Pillow 12.3.0＋真实 HTTPS C/V Gate；`/baseline` 仅 `WithPostImages`，`/reuse` 另开 `WithPostThumbnailReuse`，Redis 未启用 | PASS（同客户端开关归因，≥30%）：8 轮，前 2 轮预热，6 轮交替配对记录；基线（上中位）546.707ms、复用 359.552ms，下降 **34.23%**；中点中位 536.679/359.338ms＝33.04%，均值 531.366/359.904ms＝32.27%；12 样本；每轮首屏可见图片实际绘制，原生 vault 每轮 6 读 0 写；测试 status=PASS。原始结果 `.zcode/req004-result.json`，设备日志 `.zcode/req004-device-run.log`。边界：仅证明当前客户端复用开关收益；REQ-004 正式“双版本完整比较”验收与 Redis 热缓存增量均未由本结果覆盖 |
| Redis A/B（服务方法） | 代码 `f82bc28`（仅文档变更，代码同 `2e8bb2f`）；WSL2 `Ubuntu-2404`；一次性 private-socket PostgreSQL 16.15；Redis 7.0.15（一次性 `127.0.0.1:6391`，requirepass，无持久化）；Pillow 12.3.0；1024×768 JPEG，种子 515151；每路每轮 5 次预热＋40 次测量，3 轮交替；两组独立运行 | PASS（测试通过；按设计报告配对分布，不设任意降幅门槛）：Redis 路径 p95 稳定在 4.90–5.74ms；PostgreSQL 复用路径 5.06–7.84ms；六组降幅 2.24%、3.07%、27.47%、19.25%、2.97%、35.47%，中位 11.16%；每次运行 `gets=136,hits=135,misses=1`，Redis 测量路径共 135 次请求。原始日志 `.zcode/req004-redis-lab.log`、`.zcode/req004-redis-lab-run2.log`。边界：服务方法级、本机 loopback，收益主要来自省去 PostgreSQL payload 读取；不含 HTTPS/设备，且 PostgreSQL p95 波动较大，不外推生产 SLO |
| 真机 Redis 端到端 | Xiaomi `24129PN74C`（Android 17／SDK 37，HyperOS V816）；客户端 APK 同 `2e8bb2f`（profile，SHA-256 `e1754fcbd70026152762d12b0640f1e8d34b735c7140ad8b513b0d156d152fa4`，未改客户端）；Go 设备夹具 `a25d8e6`；WSL2 `Ubuntu-2404`；一次性 private-socket PostgreSQL 16.15；Redis 7.0.15（`127.0.0.1:6391`，requirepass，无持久化）；Pillow 12.3.0；真实 HTTPS C/V Gate | PASS：8 轮交替；`/baseline` 仅 `WithPostImages`，`/reuse` 另开复用并挂载 Redis；首次 `/reuse` 未命中后回填，其后 7 次命中。上中位基线 560.6ms vs 复用+Redis 380.8ms，下降 **32.08%**；12 样本；每轮可见图片实际绘制，原生 vault 每轮 6 读 0 写。Redis `gets=8 hits=7 misses=1 fills_set=1`，`dbsize=1`，`keyspace_hits=8`／`keyspace_misses=1`。原始结果 `.zcode/req004-device-redis-result.json`，夹具日志 `.zcode/req004-device-redis-lab.log`。边界：A/B 为“无复用” vs “复用+Redis”，设备侧未单独隔离 Redis 增量（增量见服务方法 A/B 行）；命中发生在首轮回填之后，故本次数字仍主要体现复用开关；REQ-004 正式双版本验收与完整矩阵未覆盖 |
| 审查修复（启动探测与配置边界） | 修复 `aa031c1`；审查源 `f978be8` | `/pr-review` 修复：`Check` 以 `SET NX PXAT` 探测真实写命令并校验 `UNLINK` ACL／结果；`New` 复用 `Config.validateRuntime` 校验数据库、TTL 与队列边界，负队列不再 panic；示例 ACL 明确 `user default off`。复验：`go test -count=1 ./internal/postthumbnailcache` PASS、`-race` PASS、`go vet` PASS、`go build -mod=readonly ./...` PASS、`authprivacyruntime` PASS；一次性 private-socket PostgreSQL 16.15 lab 的缩略图/预媒体套件 PASS（含新增“Redis 命中后 PG 衍生行删除必须重建”用例） |
| CI schema 清单修复 | 修复 `c3d4212`；失败 head `f978be8` | PR #34 的 `go-postgres` 在 `TestPostSchemaPartyInventoryAndPrivateColumnsPostgres` 报 relation inventory drift：补登 `thumbnail_cache_outbox` 表与 `record_thumbnail_cache_erasure()` 触发器函数；复跑 `authlab/run-post-schema-isolated.sh` 全部 `TestPostSchema*` PASS（33.615s），`TestThumbnail|TestPreparedPostMedia` 套件 PASS（92.799s，0 FAIL） |
| CI 迁移链与授权审计修复 | 修复 `d74c2fa`；失败 head `aa031c1`／`c3d4212` | 0055 使 goose ledger 到 55：avatar/DM/feed/inbox 四个迁移测试的硬编码 54 改为 55；`privacy-role-audit.sql` 登记 `thumbnail_cache_outbox` 的运行时 SELECT/UPDATE/DELETE 与 `record_thumbnail_cache_erasure()` 私有函数，rehearsal 的受限角色审计恢复。本地一次性 lab：avatar、DM、inbox、feed 四个迁移评测 PASS；`PRERELEASE_AUDIT_SQL_TEST=1` 的 `test_role_audit_postgres` 7/7 PASS（57.741s） |
| 缓存关闭维护循环崩溃修复 | 修复 `8e3eb28`；失败 head `d74c2fa` | 本机复现 PR #34 `rehearsal` 的 `probe-lifecycle`：社区进程在缩略图维护循环 panic（`community_post_thumbnails_cleanup.go:143` 对 nil `postImages.thumbnails` 取 `.redis`），连接被拒。修复为复用未启用时 `cacheActive=false`，维护仍清理孤儿并回收过期 outbox；维护单次执行提取为 `postThumbnailMaintenancePass` 以直接测试。新增 `TestThumbnailMaintenanceRunsWithReuseDisabledPostgres`（存储开启、复用/Redis 关闭）：不 panic，25h 前 pending 事件被回收、当前事件保留。复验：本机完整 `environment.py init + rehearse` 13/13 PASS（`REHEARSE_EXIT=0`）；Linux 文件系统一次性 PostgreSQL lab 的 `TestThumbnail|TestPreparedPostMedia` 59 个测试 PASS，0 FAIL；`gofmt`/`go vet ./internal/authprivacy` PASS |

## 阻碍与下一步

- WSL2 实际发行版为 `Ubuntu-2404`，unix socket PostgreSQL 与 Go/Python fixture 已可用；此前“WSL2 不可用”的判断来自错误的发行版名，已由本轮消除。原生 Windows PostgreSQL 仍不满足 unix socket 隔离条件。
- 待完成：执行 `services/api/authlab/run-post-images-isolated.sh` 的完整矩阵（真实 PostgreSQL/Redis/Python/HTTPS、双账号、替换/隐藏/删除、撤销/代次、提交 ACK 丢失、Redis 故障与删除竞态）。Redis 服务方法 A/B 已完成两组独立复跑；真机同客户端复用 A/B、真机 Redis 端到端已完成；REQ-004 正式“双版本完整比较”30% 验收仍待按该口径单独执行。
- 产品待决：最大逻辑残留时间、每实例资源预算、目标流量分配、生产开关阈值；若要求删除与 PostgreSQL 同事务抹除 Redis 字节，则该场景不得开启，生产开关保持关闭。
- 本地提交 `176b1f0`、`2e8bb2f`、`a25d8e6`、`f978be8`、`aa031c1`、`c3d4212`、`d74c2fa`、`8e3eb28`、`5781539` 已形成并推送至 `develop/codex/perf/post-thumbnail-redis`；PR #34 已创建、完成审查修复并 squash 合并到 `main`（合并提交 `803df8e`）。合并前最终 head `5781539` 的 26 项 checks：25 项 success，`actual-process-acceptance` 按设计 skipped；`rehearsal`、`go-postgres`、`gate-postgres`、`search-postgres-100k`、`flutter-and-android` 均通过。未部署，生产开关保持关闭。
