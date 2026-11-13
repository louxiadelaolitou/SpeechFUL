# Windows / WSL App 后端工具

`speechful-backend.ps1` 管理已有的两个 systemd 服务，供 IDEA External Tools 或 PowerShell 调用。它不安装服务，不接管 Frp / Cloudflare，不修改数据库、Gate 或认证策略。

```powershell
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "<工具目录>\speechful-backend.ps1" -Action Status -Config "<私有目录>\backend.private.json"
```

`-Action` 为 `Start`、`Stop`、`Status`、`Logs`。执行策略只对当前进程生效。私有配置示例（不要提交真实配置）：

```json
{
  "distro": "Your-Ubuntu",
  "user": "youruser",
  "backendUnit": "your-backend.service",
  "proxyUnit": "your-proxy.service",
  "communityPort": 3001,
  "verifierPort": 3002,
  "diagnosticsScript": "/absolute/path/upload_observability.py",
  "runtimeRoot": "/home/youruser/your-runtime"
}
```

`Status` 检查本机两个服务和健康页；PASS 不代表公网、大图上传、登录或完整发帖通过。检查未运行的 WSL 时不会主动启动它。`Start` 只做正常启动，失败时明确返回非零退出码，绝不自动恢复冻结 Gate。`Stop` 停业务与代理，不停止隧道，只关闭工具自己记录且身份一致的 WSL keeper。

公网访问需要业务后端与 Frp 两者都运行。打开 Frp 启动器后，还要确认社区、认证两条隧道启动成功。Flutter 的 `--dart-define` 是构建参数；修改 IDEA 参数不会改变手机原安装包。统一构建新包前应记录实际 C/V 地址，不把配置文件已修改当作手机切换成功。

## 上传排障

已有“查看后端日志”按钮调用 `Logs`，输出服务日志以及上传专用诊断。诊断按固定路由名展示提交帖子、查询状态、获取许可、上传原图的时间、入口域名类别、HTTP 状态、请求长度、完成标记、upstream 状态和耗时。入口类别来自代理收到的 Host，只能证明所用域名；需结合隧道配置判断完整网络路径。

没有记录不代表成功：日志从启用后开始，只在请求结束时写入；手机未发到代理、仍在上传、日志保留到期或系统日志限流都可能导致缺记录。`BODY_BUFFERED` 只是 Nginx 写临时文件的警告，不证明收齐、成功或失败。`request_length` 含请求行和头部，不能当作精确图片字节数。已收到图片请求的 204 回执也不等于最终帖子发布成功。

日志仅发往本机 `/dev/log` 的 `speechful_upload` tag，沿用系统 journal 的留存、轮换和限流，不开放管理端口、不保存正文、Authorization、客户端地址或完整资源 URL。实现使用 Nginx 官方 [syslog](https://nginx.org/en/docs/syslog.html) 与 [条件日志](https://nginx.org/en/docs/http/ngx_http_log_module.html)；系统 journal 不作为审计完整性保证。

部署所有者启用前检查真实配置、服务归属与日志留存；安装只接受已审阅配置的 SHA256，备份原配置、`nginx -t` 后平滑 reload。适配当前 C/V 双 server 布局，其他布局拒绝自动修改。原超时、路由、TLS 与业务权限不变。

```sh
python3 -B upload_observability.py install --config /absolute/proxy/nginx.conf --nginx /absolute/nginx --backup /absolute/new-backup-directory --expected-sha256 <已核实摘要> --old-host old.example --frp-host new.example
python3 -B upload_observability.py report --root /home/youruser/your-runtime
```

`report` 需要 journal 读取权限；IDEA 工具只在执行 journal 查询时使用已有 WSL root 能力。回滚由所有者确认当前配置仍为安装后的版本，再恢复备份、语法校验并 reload；不要覆盖并发运维修改。

## 启动与 Redis 维护辅助

`patch_host_startup_cleanup.py` 将已有实验 host 的退出处理提前到启动前，早期失败也有序关闭自己拥有的资源；不自动恢复 Gate。

`repair_aof_replay_acl.py` 是专门的离线维护工具，日常启动不调用。针对旧 Redis AOF 回放权限问题，操作前须有明确授权并核对备份。它只允许修改禁用且无密码的 `default` 回放用户，读取既有业务用户的 key scope，保留其 ACL 行；只接受已知 AOF 命令，拒绝运行中的目标 Redis，先复制完整目录。参考 Redis 官方 [问题记录](https://github.com/redis/redis/issues/14156) 与 [修复](https://github.com/redis/redis/pull/14545)。不对未知实例自动修复或升级。

```sh
python3 repair_aof_replay_acl.py /absolute/redis.conf /absolute/new-backup --target-user default --scope-from-user existing_business_user
python3 -B -m unittest test_aof_replay_acl test_host_startup_cleanup test_upload_observability -v
```

`test_aof_replay_integration.py` 仅创建并清理自己新建的 Linux 临时 Redis；通过 `REDIS_SERVER`、`REDIS_CLI` 指定已有二进制，不连接业务库。
