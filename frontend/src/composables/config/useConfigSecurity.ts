import { ref, computed } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import { pb } from '../pb/tracker_pb.js';
import type { WrapperRule, SecurityRulePreset, SyscallGroup, ExternalRuleSource } from '../types/config';

type CgroupSandboxActionResponse = Record<string, unknown> & {
  ip?: string;
};

type CgroupSandboxSuccessText = string | ((data: CgroupSandboxActionResponse) => string);

// ── Expanded Security Rule Presets (~38 rules from 7+ agent sources) ──
export const quickRulePresets: SecurityRulePreset[] = [
  // ── Cross-agent baseline (highest priority destructive) ──
  { comm: 'rm', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: '递归删除、删除根目录或其他高风险删除操作' },
  { comm: 'mkfs', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: '格式化磁盘 / 文件系统' },
  { comm: 'dd', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: '原始磁盘复制、覆盖或破坏性写入' },
  { comm: 'kill', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: '强制终止进程或进程组' },
  { comm: 'pkill', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: '按模式强制终止进程' },
  { comm: 'eval', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: '动态执行 shell / 代码片段' },
  { comm: 'chmod', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: '世界可写、递归权限变更等高风险权限修改' },
  { comm: 'chown', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: '递归改 owner / root 归属变更' },
  { comm: 'systemctl', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: '停止、禁用或掩蔽系统服务' },
  { comm: 'git', action: 'ALERT', priority: 180, source: 'Gemini CLI / Codex CLI', summary: '对 git 操作（含 clone, push --force, reset --hard）提醒确认' },
  { comm: 'curl', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: 'curl | bash 一类的下载即执行模式' },
  { comm: 'bash', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: 'bash -c / shell payload 执行' },
  { comm: 'sudo', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: '特权提升 / 提权执行' },

  // ── Codex CLI (sandbox + ExecPolicyManager) ──
  { comm: 'find', action: 'ALERT', priority: 190, source: 'Codex CLI', summary: 'find -exec/-delete/-fls/-fprint — 高风险查找操作，可执行任意命令' },
  { comm: 'base64', action: 'ALERT', priority: 170, source: 'Codex CLI', summary: 'base64 -o/--output — 潜在的数据外泄或文件覆写' },
  { comm: 'rg', action: 'ALERT', priority: 170, source: 'Codex CLI', summary: 'rg --pre/--hostname-bin/--search-zip — 自定义命令/外部工具执行' },
  { comm: 'pwsh', action: 'ALERT', priority: 190, source: 'Codex CLI', summary: 'PowerShell — 带有副作用的 cmdlet 执行' },
  { comm: 'powershell', action: 'ALERT', priority: 190, source: 'Codex CLI', summary: 'PowerShell — Windows shell 执行，可能绕过简单模式匹配' },

  // ── Claude Code (bash deny rules + permission hardening) ──
  { comm: 'env', action: 'ALERT', priority: 150, source: 'Claude Code', summary: 'env 包装器 — 可能用于绕过命令黑名单或泄露环境变量' },
  { comm: 'watch', action: 'ALERT', priority: 150, source: 'Claude Code', summary: 'watch 包装器 — 重复执行命令，可能放大破坏性操作' },
  { comm: 'ionice', action: 'ALERT', priority: 150, source: 'Claude Code', summary: 'ionice — I/O 优先级操纵，可能隐藏恶意 I/O 活动' },
  { comm: 'setsid', action: 'ALERT', priority: 150, source: 'Claude Code', summary: 'setsid — 从终端分离进程，绕过进程组管理' },
  { comm: 'nc', action: 'BLOCK', priority: 200, source: 'Claude Code', summary: 'nc (netcat) — 反向 shell / 网络后门 / 未授权监听' },
  { comm: 'nmap', action: 'BLOCK', priority: 200, source: 'Claude Code', summary: 'nmap — 端口扫描 / 网络侦察' },

  // ── Cursor (BLOCKED_COMMANDS + isDangerousCommand) ──
  { comm: 'mv', action: 'ALERT', priority: 170, source: 'Cursor', summary: 'mv — 移动/覆盖关键路径文件' },
  { comm: 'cp', action: 'ALERT', priority: 170, source: 'Cursor', summary: 'cp — 复制/覆盖关键系统文件' },
  { comm: 'apt-get', action: 'ALERT', priority: 160, source: 'Cursor', summary: 'apt-get — 无用户确认安装系统包' },
  { comm: 'yum', action: 'ALERT', priority: 160, source: 'Cursor', summary: 'yum — 无用户确认安装系统包' },
  { comm: 'npm', action: 'ALERT', priority: 160, source: 'Cursor', summary: 'npm i — 安装可能带有供应链风险的包' },
  { comm: 'pip', action: 'ALERT', priority: 160, source: 'Cursor', summary: 'pip install — 安装可能带有供应链风险的包' },
  { comm: 'sh', action: 'ALERT', priority: 180, source: 'Cursor', summary: 'sh — Shell 调用（类似于 bash）' },
  { comm: 'zsh', action: 'ALERT', priority: 180, source: 'Cursor', summary: 'zsh — Shell 调用' },
  { comm: 'open', action: 'ALERT', priority: 160, source: 'Cursor', summary: 'open — macOS 上启动应用程序或打开文件/URL' },
  { comm: 'start', action: 'ALERT', priority: 160, source: 'Cursor', summary: 'start — Windows 上启动进程或打开文件' },
  { comm: 'ssh', action: 'ALERT', priority: 170, source: 'Cursor', summary: 'ssh — 远程 shell / 网络隧道 / 出站连接' },
  { comm: 'killall', action: 'BLOCK', priority: 200, source: 'Cursor', summary: 'killall — 批量终止进程' },
  { comm: 'reboot', action: 'BLOCK', priority: 200, source: 'Cursor', summary: 'reboot — 重启系统' },
  { comm: 'shutdown', action: 'BLOCK', priority: 200, source: 'Cursor', summary: 'shutdown — 关闭系统' },

  // ── Amazon Q incident (cloud resource destruction) ──
  { comm: 'aws', action: 'ALERT', priority: 190, source: 'Amazon Q incident', summary: 'aws ec2 terminate / s3 rm / iam delete — 云资源破坏性操作' },

  // ── General shell/security critical ──
  { comm: 'wget', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: 'wget -O — 将文件下载到磁盘，可能为恶意 payload' },
  { comm: 'chroot', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: 'chroot — 容器/监狱逃逸' },
  { comm: 'insmod', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: 'insmod — 加载内核模块' },
  { comm: 'modprobe', action: 'BLOCK', priority: 200, source: 'Cross-agent baseline', summary: 'modprobe — 加载内核模块' },
  { comm: 'iptables', action: 'ALERT', priority: 180, source: 'Cross-agent baseline', summary: 'iptables — 防火墙规则操纵（清空/放行/重定向）' },
  { comm: 'crontab', action: 'ALERT', priority: 170, source: 'Cross-agent baseline', summary: 'crontab -e — 计划任务持久化' },
  { comm: 'usermod', action: 'ALERT', priority: 170, source: 'Cross-agent baseline', summary: 'usermod — 用户账户操纵（提权/添加组）' },
  { comm: 'passwd', action: 'ALERT', priority: 170, source: 'Cross-agent baseline', summary: 'passwd — 非交互式脚本中修改密码' },
  { comm: 'gcloud', action: 'ALERT', priority: 190, source: 'Cross-agent baseline', summary: 'gcloud — GCP 云资源删除/修改操作' },

  // ── Claude Code Safety Net (git/rm/find/xargs deep analysis) ──
  { comm: 'xargs', action: 'ALERT', priority: 190, source: 'Claude Code Safety Net', summary: 'xargs — 批量管道执行，可能将不可信输入传递给危险命令' },
  { comm: 'git', action: 'ALERT', priority: 175, source: 'Claude Code Safety Net', summary: 'git reset --hard / clean -f / push --force 等破坏性操作需人工确认' },
  { comm: 'rm', action: 'BLOCK', priority: 195, source: 'Claude Code Safety Net', summary: 'rm -rf 根/家目录或超出 cwd 范围始终阻止；/tmp 允许' },
  { comm: 'watch', action: 'ALERT', priority: 170, source: 'Claude Code Safety Net', summary: 'watch — 重复执行命令，可能放大破坏性操作的影响' },
  { comm: 'ionice', action: 'ALERT', priority: 160, source: 'Claude Code Safety Net', summary: 'ionice — I/O 优先级操纵，可能隐藏恶意 I/O 活动' },
  { comm: 'setsid', action: 'ALERT', priority: 170, source: 'Claude Code Safety Net', summary: 'setsid — 从终端分离进程，绕过进程组管理和信号控制' },
  { comm: 'mount', action: 'ALERT', priority: 175, source: 'Claude Code Safety Net', summary: 'mount --bind — bind mount 可能用于容器/jail 逃逸' },
];

// ── External rule sources for one-click import ──
export const externalRuleSources: ExternalRuleSource[] = [
  {
    id: 'secure-code-warrior',
    name: 'Secure Code Warrior AI Security Rules',
    description: '社区驱动的 AI 代理安全规则集，覆盖常见编码安全问题',
    url: 'https://raw.githubusercontent.com/SecureCodeWarrior/ai-security-rules/main/rules.json',
    format: 'json',
    sourceAttribution: 'github.com/SecureCodeWarrior/ai-security-rules',
    category: 'community',
  },
  {
    id: 'claude-code-safety-net',
    name: 'Claude Code Safety Net',
    description: '针对 Claude Code / Copilot 等 AI 编码代理的社区安全钩子规则',
    url: 'https://raw.githubusercontent.com/kenryu42/claude-code-safety-net/main/rules.json',
    format: 'json',
    sourceAttribution: 'github.com/kenryu42/claude-code-safety-net',
    category: 'community',
  },
  {
    id: 'owasp-agentic',
    name: 'OWASP Agentic AI Security Guidelines',
    description: 'OWASP 针对 AI 代理安全的 Top 10 指导方针（固定预设，非远程获取）',
    url: '',
    format: 'markdown',
    sourceAttribution: 'owasp.org',
    category: 'owasp',
  },
];

// OWASP presets (not machine-readable, provided as fixed data)
export const owaspPresets: SecurityRulePreset[] = [
  { comm: 'aws', action: 'ALERT', priority: 190, source: 'OWASP Agentic AI', summary: '避免 AI 代理执行云资源破坏性操作' },
  { comm: 'gcloud', action: 'ALERT', priority: 190, source: 'OWASP Agentic AI', summary: '避免 AI 代理执行云资源破坏性操作' },
  { comm: 'az', action: 'ALERT', priority: 190, source: 'OWASP Agentic AI', summary: '避免 AI 代理执行 Azure 资源破坏性操作' },
  { comm: 'terraform', action: 'ALERT', priority: 180, source: 'OWASP Agentic AI', summary: 'terraform destroy / apply 需人工确认' },
  { comm: 'kubectl', action: 'ALERT', priority: 180, source: 'OWASP Agentic AI', summary: 'kubectl delete 需人工确认，防止集群资源误删' },
];

// ── Syscall Groups ──
export const syscallGroups: SyscallGroup[] = [
  {
    key: 'file', title: 'File Operations', icon: 'file', color: '#1677ff',
    syscalls: [
      { type: pb.EventType.OPENAT, name: 'openat', desc: 'Open file (fd-relative)' },
      { type: pb.EventType.OPEN, name: 'open', desc: 'Open file' },
      { type: pb.EventType.READ, name: 'read', desc: 'Read from file descriptor' },
      { type: pb.EventType.WRITE, name: 'write', desc: 'Write to file descriptor' },
      { type: pb.EventType.MKDIR, name: 'mkdirat', desc: 'Create directory' },
      { type: pb.EventType.UNLINK, name: 'unlinkat', desc: 'Delete file/directory' },
      { type: pb.EventType.CHMOD, name: 'chmod', desc: 'Change file permissions' },
      { type: pb.EventType.CHOWN, name: 'chown', desc: 'Change file ownership' },
      { type: pb.EventType.RENAME, name: 'rename', desc: 'Rename / move file' },
      { type: pb.EventType.LINK, name: 'link', desc: 'Create hard link' },
      { type: pb.EventType.SYMLINK, name: 'symlink', desc: 'Create symbolic link' },
      { type: pb.EventType.MKNOD, name: 'mknod', desc: 'Create device node' },
    ],
  },
  {
    key: 'network', title: 'Network Operations', icon: 'global', color: '#722ed1',
    syscalls: [
      { type: pb.EventType.NETWORK_CONNECT, name: 'connect', desc: 'Outgoing TCP/UDP connection' },
      { type: pb.EventType.NETWORK_BIND, name: 'bind', desc: 'Bind socket to address/port' },
      { type: pb.EventType.NETWORK_SENDTO, name: 'sendto', desc: 'Send data to socket' },
      { type: pb.EventType.NETWORK_RECVFROM, name: 'recvfrom', desc: 'Receive data from socket' },
      { type: pb.EventType.SOCKET, name: 'socket', desc: 'Create socket endpoint' },
      { type: pb.EventType.ACCEPT, name: 'accept', desc: 'Accept incoming connection' },
      { type: pb.EventType.ACCEPT4, name: 'accept4', desc: 'Accept4 connection (with flags)' },
    ],
  },
  {
    key: 'process', title: 'Process Operations', icon: 'thunderbolt', color: '#fa8c16',
    syscalls: [
      { type: pb.EventType.EXECVE, name: 'execve', desc: 'Execute a new program' },
      { type: pb.EventType.CLONE, name: 'clone', desc: 'Create child process/thread' },
      { type: pb.EventType.EXIT, name: 'exit', desc: 'Terminate process' },
    ],
  },
  {
    key: 'device', title: 'Device Operations', icon: 'control', color: '#f5222d',
    syscalls: [
      { type: pb.EventType.IOCTL, name: 'ioctl', desc: 'Device I/O control operation' },
    ],
  },
  {
    key: 'security', title: 'Security Critical', icon: 'safety', color: '#cf1322',
    syscalls: [
      { type: 25, name: 'kill / tkill / tgkill', desc: 'Send signal to process' },
      { type: 25, name: 'ptrace', desc: 'Trace/debug another process' },
      { type: 25, name: 'prctl', desc: 'Process control operations' },
      { type: 25, name: 'seccomp', desc: 'Secure computing (sandbox)' },
      { type: 25, name: 'bpf', desc: 'Load/manage eBPF programs' },
      { type: 25, name: 'init_module', desc: 'Load kernel module' },
      { type: 25, name: 'kexec_load / kexec_file_load', desc: 'Load new kernel image' },
      { type: 25, name: 'iopl / ioperm', desc: 'I/O privilege level change' },
      { type: 25, name: 'capget / capset', desc: 'Get/set process capabilities' },
      { type: 25, name: 'syslog', desc: 'Read kernel message buffer' },
      { type: 25, name: 'setns / unshare', desc: 'Switch/unshare namespace' },
      { type: 25, name: 'process_vm_readv / writev', desc: 'Read/write remote process memory' },
      { type: 25, name: 'kcmp', desc: 'Compare kernel objects' },
      { type: 25, name: 'request_key / keyctl', desc: 'Kernel key management' },
    ],
  },
];

export function useConfigSecurity() {
  // ── Wrapper Rules State ──
  const wrapperRules = ref<Record<string, WrapperRule>>({});
  const newRuleComm = ref('');
  const newRuleAction = ref('BLOCK');
  const newRuleRewritten = ref('');
  const newRuleRegex = ref('');
  const newRuleReplacement = ref('');
  const newRulePriority = ref(0);
  const previewTestInput = ref('');

  // ── Syscall Interception State ──
  const disabledEventTypes = ref<Set<number>>(new Set());

  // ── Kernel / cgroup enforcement state ──
  const cgroupSandboxStatus = ref({
    available: false,
    attached: false,
    cgroupPath: '',
    linkPins: [] as string[],
    blockedCgroups: [] as string[],
    blockedIPs: [] as string[],
    blockedPorts: [] as number[],
    maps: {
      cgroupBlocklist: false,
      ipBlocklist: false,
      ip6Blocklist: false,
      portBlocklist: false,
      stats: false,
    },
    stats: {
      connectChecked: 0,
      connectBlocked: 0,
      connectAllowed: 0,
      checked: 0,
      blocked: 0,
      allowed: 0,
    },
    statsError: '',
    error: '',
  });
  const cgroupSandboxLoading = ref(false);
  const cgroupTargetID = ref('');
  const cgroupTargetPID = ref<number | null>(null);
  const cgroupTargetIP = ref('');
  const cgroupTargetPort = ref<number | null>(4444);

  // ── Kernel / BPF LSM enforcement state ──
  const lsmEnforcerStatus = ref({
    available: false,
    attached: false,
    linkPins: [] as string[],
    maps: {
      execPathBlocklist: false,
      execNameBlocklist: false,
      fileNameBlocklist: false,
      stats: false,
    },
    blockedExecPaths: [] as string[],
    blockedExecNames: [] as string[],
    blockedFileNames: [] as string[],
    stats: {
      execChecked: 0,
      execBlocked: 0,
      fileChecked: 0,
      fileBlocked: 0,
    },
    statsError: '',
    error: '',
  });
  const lsmEnforcerLoading = ref(false);
  const lsmExecPath = ref('/usr/bin/nc');
  const lsmExecName = ref('nc');
  const lsmFileName = ref('id_rsa');

  // ── External Rule Import State ──
  const fetchedExternalRules = ref<WrapperRule[]>([]);
  const fetchSourceLoading = ref<string | null>(null);
  const importingExternalRules = ref(false);

  // ── Wrapper Rules CRUD ──
  const fetchRules = async () => {
    try {
      const res = await axios.get('/config/rules');
      wrapperRules.value = res.data;
    } catch (_) {}
  };

  const postRule = async (rule: WrapperRule) => {
    await axios.post('/config/rules', rule);
  };

  const buildManualRulePayload = (): WrapperRule => ({
    comm: newRuleComm.value,
    action: newRuleAction.value,
    rewritten_cmd:
      newRuleAction.value === 'REWRITE' && !newRuleRegex.value
        ? newRuleRewritten.value.split(' ').filter((s) => s)
        : [],
    regex: newRuleRegex.value,
    replacement: newRuleReplacement.value,
    priority: newRulePriority.value,
  });

  const resetRuleForm = () => {
    newRuleComm.value = '';
    newRuleRewritten.value = '';
    newRuleRegex.value = '';
    newRuleReplacement.value = '';
    newRulePriority.value = 0;
    previewTestInput.value = '';
  };

  const saveRule = async () => {
    if (!newRuleComm.value) return;
    try {
      await postRule(buildManualRulePayload());
      message.success('Rule saved');
      resetRuleForm();
      await fetchRules();
    } catch (_) {
      message.error('Failed to save rule');
    }
  };

  const deleteRule = async (comm: string) => {
    try {
      await axios.delete(`/config/rules/${comm}`);
      message.success('Rule deleted');
      fetchRules();
    } catch (_) {}
  };

  // ── Quick Presets ──
  const addQuickRulePreset = async (preset: SecurityRulePreset) => {
    try {
      await postRule({
        comm: preset.comm,
        action: preset.action,
        rewritten_cmd: [],
        priority: preset.priority,
      });
      message.success(`已添加预设：${preset.comm} → ${preset.action}`);
      await fetchRules();
    } catch (_) {
      message.error(`Failed to add preset rule: ${preset.comm}`);
    }
  };

  const addAllQuickRulePresets = async () => {
    let success = 0;
    let failed = 0;
    for (const preset of quickRulePresets) {
      try {
        await postRule({
          comm: preset.comm,
          action: preset.action,
          rewritten_cmd: [],
          priority: preset.priority,
        });
        success++;
      } catch (_) {
        failed++;
      }
    }
    await fetchRules();
    if (failed > 0) {
      message.warning(`一键添加完成：成功 ${success} 条，失败 ${failed} 条`);
    } else {
      message.success(`一键添加完成：写入/更新 ${success} 条预设规则`);
    }
  };

  // ── External Rule Import ──
  const fetchExternalRules = async (source: ExternalRuleSource) => {
    if (!source.url) {
      // OWASP: use fixed presets
      fetchedExternalRules.value = owaspPresets.map((p) => ({
        comm: p.comm,
        action: p.action,
        rewritten_cmd: [],
        priority: p.priority,
      }));
      message.info(`已加载 ${fetchedExternalRules.value.length} 条 OWASP 预设规则（本地提供）`);
      return;
    }
    fetchSourceLoading.value = source.id;
    try {
      const res = await axios.get(source.url, { timeout: 15000 });
      if (source.id === 'secure-code-warrior') {
        const rules = res.data?.rules || res.data || [];
        fetchedExternalRules.value = (Array.isArray(rules) ? rules : []).map((r: any) => ({
          comm: r.command || r.comm || r.name || '',
          action: (r.severity === 'critical' || r.action === 'BLOCK') ? 'BLOCK' : 'ALERT',
          rewritten_cmd: [],
          priority: r.priority ?? 180,
        })).filter((r: WrapperRule) => r.comm);
      } else if (source.id === 'claude-code-safety-net') {
        const rules = res.data?.rules || res.data || [];
        fetchedExternalRules.value = (Array.isArray(rules) ? rules : []).map((r: any) => ({
          comm: r.command || r.comm || r.name || '',
          action: r.action || 'ALERT',
          rewritten_cmd: [],
          priority: r.priority ?? 180,
        })).filter((r: WrapperRule) => r.comm);
      } else {
        fetchedExternalRules.value = [];
      }
      message.success(`从 ${source.name} 获取到 ${fetchedExternalRules.value.length} 条规则`);
    } catch (e: any) {
      message.error(`获取 ${source.name} 失败：${e.message || '网络错误'}`);
      fetchedExternalRules.value = [];
    } finally {
      fetchSourceLoading.value = null;
    }
  };

  const importAllFetchedRules = async () => {
    if (!fetchedExternalRules.value.length) {
      message.warning('没有可导入的规则，请先获取外部来源');
      return;
    }
    importingExternalRules.value = true;
    let success = 0;
    let failed = 0;
    for (const rule of fetchedExternalRules.value) {
      try {
        await postRule(rule);
        success++;
      } catch (_) {
        failed++;
      }
    }
    await fetchRules();
    if (failed > 0) {
      message.warning(`外部规则导入完成：成功 ${success} 条，失败 ${failed} 条`);
    } else {
      message.success(`外部规则导入完成：${success} 条全部写入`);
    }
    fetchedExternalRules.value = [];
    importingExternalRules.value = false;
  };

  // ── Syscall Toggles ──
  const fetchDisabledEventTypes = async () => {
    try {
      const res = await axios.get('/config/event-types');
      disabledEventTypes.value = new Set(res.data.disabled_event_types || []);
    } catch (_) {}
  };

  const toggleEventType = async (type: number, disabled: boolean) => {
    try {
      if (disabled) {
        await axios.delete(`/config/event-types/${type}/disable`);
      } else {
        await axios.post(`/config/event-types/${type}/disable`);
      }
      fetchDisabledEventTypes();
    } catch (_) {}
  };

  // ── Kernel / cgroup enforcement ──
  const fetchCgroupSandboxStatus = async () => {
    cgroupSandboxLoading.value = true;
    try {
      const res = await axios.get('/sandbox/cgroup/status');
      cgroupSandboxStatus.value = {
        ...cgroupSandboxStatus.value,
        ...res.data,
        maps: {
          ...cgroupSandboxStatus.value.maps,
          ...(res.data?.maps || {}),
        },
        stats: {
          ...cgroupSandboxStatus.value.stats,
          ...(res.data?.stats || {}),
        },
        blockedCgroups: res.data?.blockedCgroups || [],
        blockedIPs: res.data?.blockedIPs || [],
        blockedPorts: res.data?.blockedPorts || [],
      };
    } catch (e: any) {
      message.error(`加载 cgroup sandbox 状态失败：${e.response?.data?.error || e.message || 'unknown error'}`);
    } finally {
      cgroupSandboxLoading.value = false;
    }
  };

  const postCgroupSandboxAction = async (path: string, payload: Record<string, unknown>, successText: CgroupSandboxSuccessText) => {
    cgroupSandboxLoading.value = true;
    try {
      const res = await axios.post<CgroupSandboxActionResponse>(path, payload);
      const successMessage = typeof successText === 'function' ? successText(res.data || {}) : successText;
      message.success(successMessage);
      await fetchCgroupSandboxStatus();
    } catch (e: any) {
      message.error(e.response?.data?.error || 'cgroup sandbox 操作失败；请确认 /config/runtime 已启用 policy management');
    } finally {
      cgroupSandboxLoading.value = false;
    }
  };

  const blockCgroupID = async () => {
    const cgroupId = cgroupTargetID.value.trim();
    if (!/^[1-9]\d*$/.test(cgroupId)) {
      message.warning('请输入有效的 cgroup id');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/block-cgroup', { cgroupId }, `已阻断 cgroup ${cgroupId} 的出站连接`);
  };

  const unblockCgroupID = async () => {
    const cgroupId = cgroupTargetID.value.trim();
    if (!/^[1-9]\d*$/.test(cgroupId)) {
      message.warning('请输入有效的 cgroup id');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/unblock-cgroup', { cgroupId }, `已解除 cgroup ${cgroupId} 的出站阻断`);
  };

  const blockCgroupPID = async () => {
    if (!cgroupTargetPID.value || cgroupTargetPID.value <= 0) {
      message.warning('请输入有效的 PID');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/block-pid', { pid: cgroupTargetPID.value }, `已阻断 PID ${cgroupTargetPID.value} 所在 cgroup 的出站连接`);
  };

  const unblockCgroupPID = async () => {
    if (!cgroupTargetPID.value || cgroupTargetPID.value <= 0) {
      message.warning('请输入有效的 PID');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/unblock-pid', { pid: cgroupTargetPID.value }, `已解除 PID ${cgroupTargetPID.value} 所在 cgroup 的出站阻断`);
  };

  const blockCgroupIP = async () => {
    const ip = cgroupTargetIP.value.trim();
    if (!ip) {
      message.warning('请输入 IPv4、IPv6 或 IPv4-mapped IPv6 地址');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/block-ip', { ip }, (data) => `已在内核层阻断 ${data.ip || ip}`);
  };

  const unblockCgroupIP = async () => {
    const ip = cgroupTargetIP.value.trim();
    if (!ip) {
      message.warning('请输入 IPv4、IPv6 或 IPv4-mapped IPv6 地址');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/unblock-ip', { ip }, (data) => `已解除 ${data.ip || ip} 的内核层阻断`);
  };

  const blockCgroupPort = async () => {
    if (!cgroupTargetPort.value) {
      message.warning('请输入端口');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/block-port', { port: cgroupTargetPort.value }, `已在内核层阻断端口 ${cgroupTargetPort.value}`);
  };

  const unblockCgroupPort = async () => {
    if (!cgroupTargetPort.value) {
      message.warning('请输入端口');
      return;
    }
    await postCgroupSandboxAction('/sandbox/cgroup/unblock-port', { port: cgroupTargetPort.value }, `已解除端口 ${cgroupTargetPort.value} 的内核层阻断`);
  };

  // ── Kernel / BPF LSM enforcement ──
  const fetchLsmEnforcerStatus = async () => {
    lsmEnforcerLoading.value = true;
    try {
      const res = await axios.get('/sandbox/lsm/status');
      lsmEnforcerStatus.value = {
        ...lsmEnforcerStatus.value,
        ...res.data,
        maps: {
          ...lsmEnforcerStatus.value.maps,
          ...(res.data?.maps || {}),
        },
        stats: {
          ...lsmEnforcerStatus.value.stats,
          ...(res.data?.stats || {}),
        },
        blockedExecPaths: res.data?.blockedExecPaths || [],
        blockedExecNames: res.data?.blockedExecNames || [],
        blockedFileNames: res.data?.blockedFileNames || [],
      };
    } catch (e: any) {
      message.error(`加载 BPF LSM 状态失败：${e.response?.data?.error || e.message || 'unknown error'}`);
    } finally {
      lsmEnforcerLoading.value = false;
    }
  };

  const postLsmEnforcerAction = async (path: string, payload: Record<string, unknown>, successText: string) => {
    lsmEnforcerLoading.value = true;
    try {
      await axios.post(path, payload);
      message.success(successText);
      await fetchLsmEnforcerStatus();
    } catch (e: any) {
      message.error(e.response?.data?.error || 'BPF LSM 操作失败；请确认内核启用了 BPF LSM 且 /config/runtime 已启用 policy management');
    } finally {
      lsmEnforcerLoading.value = false;
    }
  };

  const blockLsmExecPath = async () => {
    const path = lsmExecPath.value.trim();
    if (!path) {
      message.warning('请输入要拦截的执行路径');
      return;
    }
    await postLsmEnforcerAction('/sandbox/lsm/block-exec-path', { path }, `已在 BPF LSM 阻断执行：${path}`);
  };

  const unblockLsmExecPath = async (path = lsmExecPath.value.trim()) => {
    if (!path) {
      message.warning('请输入要解除的执行路径');
      return;
    }
    await postLsmEnforcerAction('/sandbox/lsm/unblock-exec-path', { path }, `已解除 BPF LSM 执行阻断：${path}`);
  };

  const blockLsmExecName = async () => {
    const name = lsmExecName.value.trim();
    if (!name) {
      message.warning('请输入要拦截的可执行文件名');
      return;
    }
    await postLsmEnforcerAction('/sandbox/lsm/block-exec-name', { name }, `已在 BPF LSM 阻断可执行文件名：${name}`);
  };

  const unblockLsmExecName = async (name = lsmExecName.value.trim()) => {
    if (!name) {
      message.warning('请输入要解除的可执行文件名');
      return;
    }
    await postLsmEnforcerAction('/sandbox/lsm/unblock-exec-name', { name }, `已解除 BPF LSM 可执行文件名阻断：${name}`);
  };

  const blockLsmFileName = async () => {
    const name = lsmFileName.value.trim();
    if (!name) {
      message.warning('请输入要拦截的文件或目录 basename');
      return;
    }
    await postLsmEnforcerAction('/sandbox/lsm/block-file-name', { name }, `已在 BPF LSM 阻断打开/读写/mmap/mprotect/setattr/创建/link/symlink/删除/mkdir/rmdir/mknod/rename basename：${name}`);
  };

  const unblockLsmFileName = async (name = lsmFileName.value.trim()) => {
    if (!name) {
      message.warning('请输入要解除的文件或目录 basename');
      return;
    }
    await postLsmEnforcerAction('/sandbox/lsm/unblock-file-name', { name }, `已解除 BPF LSM 打开/读写/mmap/mprotect/setattr/创建/link/symlink/删除/mkdir/rmdir/mknod/rename basename 阻断：${name}`);
  };

  // ── Regex Preview ──
  const regexPreviewResult = computed(() => {
    if (!newRuleRegex.value || !previewTestInput.value) return '';
    try {
      const re = new RegExp(newRuleRegex.value);
      return previewTestInput.value.replace(re, newRuleReplacement.value);
    } catch (_) {
      return 'Invalid Regex';
    }
  });

  return {
    wrapperRules,
    newRuleComm, newRuleAction, newRuleRewritten,
    newRuleRegex, newRuleReplacement, newRulePriority, previewTestInput,
    disabledEventTypes,
    cgroupSandboxStatus, cgroupSandboxLoading,
    cgroupTargetID, cgroupTargetPID, cgroupTargetIP, cgroupTargetPort,
    lsmEnforcerStatus, lsmEnforcerLoading,
    lsmExecPath, lsmExecName, lsmFileName,
    fetchedExternalRules, fetchSourceLoading, importingExternalRules,
    fetchRules, postRule, saveRule, deleteRule,
    buildManualRulePayload, resetRuleForm,
    addQuickRulePreset, addAllQuickRulePresets,
    fetchExternalRules, importAllFetchedRules,
    fetchDisabledEventTypes, toggleEventType,
    fetchCgroupSandboxStatus,
    blockCgroupID, unblockCgroupID,
    blockCgroupPID, unblockCgroupPID,
    blockCgroupIP, unblockCgroupIP,
    blockCgroupPort, unblockCgroupPort,
    fetchLsmEnforcerStatus,
    blockLsmExecPath, unblockLsmExecPath,
    blockLsmExecName, unblockLsmExecName,
    blockLsmFileName, unblockLsmFileName,
    regexPreviewResult,
  };
}
