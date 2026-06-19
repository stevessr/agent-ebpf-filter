# 策略语义

策略语义必须精确表达，不能为了展示效果夸大能力范围。

## tracked commands

`tracked_comms`：

- 16-byte command-name key；
- exact match；
- 用于标记 / 追踪 comm；
- 不是通配符规则。

## tracked paths

`tracked_paths`：

- 256-byte path key；
- exact match；
- 不是递归目录树；
- 不代表所有子路径都被跟踪。

如果存在 prefix 语义，应明确区分 tracked path 与 tracked prefix。

## Wrapper rules

Wrapper 通过 `/tmp/agent-ebpf.sock` 询问 backend policy engine。响应：

- `ALLOW`：继续执行；
- `BLOCK`：打印并退出；
- `ALERT`：打印告警后继续；
- `REWRITE`：替换命令和参数后执行。

Wrapper 不是完整 sandbox，它只覆盖经 wrapper 启动的命令。

## cgroup destination blocking

cgroup sandbox 支持：

- exact cgroup id；
- exact IPv4；
- exact IPv6；
- exact TCP/UDP destination port。

不是：

- CIDR；
- IP range；
- domain name policy；
- L7 firewall。

## BPF LSM file / exec policy

Exec：

- exact executable path；
- executable basename。

File / directory：

- basename-based；
- 覆盖 open、permission、mmap、mprotect、setattr、create、link、symlink、unlink、mkdir、rmdir、mknod、rename。

不是：

- recursive directory policy；
- glob pattern；
- full filesystem sandbox。

## Policy mutation

修改策略需要：

1. 对应 feature compiled in；
2. release mode token；
3. runtime policy management enabled；
4. 后端 API 写入 map / store；
5. UI / status 可观测。
