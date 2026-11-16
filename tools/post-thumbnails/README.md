# 私有帖子缩略图复用

状态与验收证据集中在[需求004](../../docs/tasks/REQ-004-PROTECTED-THUMBNAILS.md)。

先用现有 Goose 流程执行前向迁移0043，再运行 `infra/postgres/grant-post-images.sql`。无存量全量转码。按既有部署流程同步可信 `validate_image.py` 及实际SHA-256，重启时显式配置：

```json
{"posts":{"preparedMediaReads":true},"postImages":{"thumbnailReuse":true}}
```

这只是现有完整配置中的新增字段，不能替代用途密钥、Gate、数据库和Python路径等原配置。运行角色不能更改256MiB衍生payload预算；迁移所有者可按容量规划调整 `c_posts.thumbnail_storage.capacity_bytes`，不得低于当前 `used_bytes`。首次持久化也在最终授权事务内；并发／队列分别最多2／32，单个共享准备任务最多20秒。请求取消不取消其他等待者，私有结果仍须逐请求最终授权。

默认按访问生成。确需提前生成一批已发布图片时，将帖子UUID每行一条写入本地清单，用已有合法会话通过受保护HTTPS接口预热：

```sh
chmod 600 /task-owned/session-token
python3 -B tools/post-thumbnails/backfill.py \
  --origin https://approved-c.example \
  --token-file /task-owned/session-token \
  --post-ids /task-owned/post-ids.txt \
  --ca-file /task-owned/private-ca.pem \
  --interval 1 --max-posts 100
```

工具只请求当前会话有权查看的清单和thumbnail，不读取或落盘原图／缩略字节，不跟随重定向。每次请求间隔至少指定秒数，Ctrl+C／SIGTERM停止后续请求；当前请求使用15秒网络超时。遇401／403／429／503停止，404计数后继续；输出仅有聚合计数，停止退出码为2。它不查询全部用户图片，不绕过最终Gate，不获得新的权限，也不迁移数据库。

后台每分钟清理最多128条孤儿或容量压力下的最旧记录。临时隐藏会清理该帖衍生图，恢复后按访问重新生成。关闭 `thumbnailReuse` 回到现有受保护转码路径，后台和数据库擦除钩子继续运行；0043的Down拒绝移除生命周期保护。原图及客户端租约、保存、no-store不变。

服务端性能验证：固定1024×768确定性JPEG、Go1.22／PostgreSQL16／Pillow12.3.0及相同Gate环境；两路都启用需求001准备读取，只有复用开关不同。显式设置 `POST_THUMBNAIL_PERFORMANCE=1`，由一次性数据库runner执行 `TestThumbnailReusePerformancePostgres`。每轮每路预热5次、测量40次，共三组配对，次序交替；逐轮p95下降至少60%，不丢弃失败轮次。Go与RAW时钟不一致、请求失败或字节变化都使测量失败。冷生成耗时单列。

设备30%正式验收按用户确认比较需求004修改前后的完整版本，使用真实首页首屏实际可见图片，固定设备、网络、内容、列表模式和滚动位置；新服务端预先生成小图，两版客户端清空图片内存缓存。从相同页面加载起点到首屏可见图片完成绘制，交替新旧版本测量并保留全部样本、冷生成数据和失败；不能拼接不同时间段的结果报PASS。当前integration test仅作同客户端开关归因，其结果另列。未接手机、只运行HTTP或服务方法不能代替设备PASS；不要求屏幕外横滑图片完成显示，也不使用“九图首屏”。
