# 🛠️ 故障排查与常见问题指南

本文件汇总了在部署、运行及测试 **Agent eBPF Filter** 过程中可能遇到的常见故障及其排除方法。

---

## 1. 后端服务故障 (Backend Issues)

### 1.1 后端服务无法启动 (Bootstrap Failures)
当运行 `go run ./app` 或启动 `agent-ebpf-filter` systemd 服务失败时，通常与特权或内核 eBPF 支持有关。

* **排查步骤**：
  1. **检查 BTF 是否可用**：本系统依赖内核 BTF 导出。运行以下命令验证：
     ```bash
     ls -la /sys/kernel/btf/vmlinux
     ```
     如果该文件不存在，说明当前内核未开启 `CONFIG_DEBUG_INFO_BTF`。建议更换支持 BTF 的现代内核（5.15+）。
  2. **检查 bpffs 挂载状况**：
     ```bash
     mount | grep bpf
     ```
     如果没有输出，请手动挂载 BPF 文件系统：
     ```bash
     sudo mount -t bpf bpffs /sys/fs/bpf
     ```
  3. **特权校验**：加载 eBPF 程序通常需要 `root` 权限（或 `CAP_BPF`、`CAP_NET_ADMIN`、`CAP_SYS_ADMIN` 能力）。请确保使用 `sudo -E` 或以 root 用户启动。
  4. **端口占用**：默认 API 监听 `8080` 端口。如果被占用，后端会自动寻找下一个可用端口（如 8081），并将其写入 `backend/.port`。请确认无其他强占冲突。

### 1.2 内核日志中报 eBPF Verifier 错误
* **原因**：由于内核版本差异，Verifier（验证器）对 eBPF 指令的约束可能导致加载失败。
* **解决办法**：
  * 检查 `/sys/kernel/debug/tracing/trace_pipe` 或运行 `dmesg -T | grep bpf` 获取具体 Verifier 报错日志。
  * 报告 issue 并附带具体的 Verifier 错误行信息。

---

## 2. 内核态硬阻断失效 (OS Enforcement Issues)

### 2.1 cgroup 网络阻断未生效
* **原因 1：cgroup v2 未启用或挂载不正确**：
  本系统的网络拦截高度依赖 cgroup v2 拓扑。运行以下命令验证：
  ```bash
  mount | grep cgroup2
  ```
  若未挂载，可运行：
  ```bash
  sudo mount -t cgroup2 cgroup2 /sys/fs/cgroup
  ```
* **原因 2：PID 对应进程未进入匹配的 cgroup**：
  eBPF 阻断根据进程所在的 cgroup2 inode 进行匹配。如果使用的是旧版本的容器化技术，可能会导致 cgroup 层次混乱。建议在标准 systemd 切片或 Kubernetes Pod 中执行验证。

### 2.2 BPF LSM 文件阻断失效
* **原因 1：内核未启用 BPF LSM**：
  运行以下命令检查当前系统的 LSM 初始化顺序中是否包含 `bpf`：
  ```bash
  cat /sys/kernel/security/lsm
  ```
  如果输出里没有 `bpf`，说明 BPF LSM 未启用。
* **解决方法**：
  编辑 `/etc/default/grub`，在 `GRUB_CMDLINE_LINUX` 中追加 `lsm=landlock,lockdown,yama,integrity,bpf`。
  然后更新 Grub 并重启系统：
  ```bash
  # Debian/Ubuntu
  sudo update-grub
  # Fedora/CentOS/RHEL
  sudo grub2-mkconfig -o /boot/grub2/grub.cfg
  sudo reboot
  ```

---

## 3. 前端工作台连接故障 (Frontend & WS Issues)

### 3.1 前端显示 "Backend Offline" 且无数据推送
* **排查步骤**：
  1. **检查代理端口文件**：
     前端开发服务器使用 Vite 代理，会读取根目录下的 `backend/.port` 端口文件。如果该文件内容被篡改或因权限问题无法读取，会导致请求转发至错误的端口。
  2. **WebSocket 握手失败**：
     打开浏览器控制台（F12），检查 WS 握手请求。如果使用了反向代理（如 Nginx），请确保配置了 WebSocket 升级报头：
     ```nginx
     proxy_set_header Upgrade $http_upgrade;
     proxy_set_header Connection "upgrade";
     ```
  3. **API 认证未通过**：
     在 release 模式下，全量接口均被 runtime token 保护。如果前端未能在 localstorage 中正确加载 token，或者后端重新生成了 token，会导致所有 WS 和 REST 请求返回 `401 Unauthorized`。
     可在 `~/.config/agent-ebpf-filter/runtime.json` 中找到当前有效的 `access_token` 进行手动设置。

---

## 4. AI CLI Hook 与 Wrapper 故障 (Integration Issues)

### 4.1 CLI 命令执行没有被拦截，没有产生 Wrapper 事件
* **排查步骤**：
  1. **检查 PATH 变量**：
     确保 `agent-wrapper` 已经放入您的全局 PATH 环境变量，并且目标 AI Agent 正在调用该 wrapper 代理可执行文件，而非直接调用底层的 shell 或原生二进制。
  2. **检查 UDS 策略套接字**：
     wrapper 通过 `/tmp/agent-ebpf.sock` 与后端同步握手。检查套接字文件是否存在，以及当前执行命令的用户是否对其拥有读写权限（默认权限为 `0600`，仅所有者和 root 可读写）。

### 4.2 运行在容器中时无法接入宿主机后端
* **排查步骤**：
  1. 容器需要将宿主机的 `/tmp/agent-ebpf.sock` 套接字挂载进容器内部。
  2. 容器也需要将自身的 PID 命名空间暴露（如在 docker 中使用 `--pid=host`），以便后端 BPF 程序能通过宿主机 PID 识别容器内部的 Agent 进程。

---

## 🔗 相关导航

- [🚀 构建与运行](build-and-run.md) —— 环境准备与编译命令
- [🛡️ 安全模型](../security/model.md) —— 权限边界说明
- [🦕 eBPF 与 OS Enforcement](../backend/ebpf-os-enforcement.md) —— 阻断脚本用法
