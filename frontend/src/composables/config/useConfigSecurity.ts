import { ref, computed } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import { pb } from "../../pb/tracker_pb.js";
import {
  externalRuleSources,
  owaspPresets,
  quickRulePresets,
  syscallGroups,
} from "./securityPresets";
export {
  externalRuleSources,
  owaspPresets,
  quickRulePresets,
  syscallGroups,
} from "./securityPresets";
import type {
  WrapperRule,
  ExternalRuleSource,
  SecurityRulePreset,
} from "../../types/config";

type CgroupSandboxActionResponse = Record<string, unknown> & {
  ip?: string;
};

type CgroupSandboxSuccessText =
  | string
  | ((data: CgroupSandboxActionResponse) => string);

export function useConfigSecurity() {
  // ── Wrapper Rules State ──
  const wrapperRules = ref<Record<string, WrapperRule>>({});
  const newRuleComm = ref("");
  const newRuleAction = ref("BLOCK");
  const newRuleRewritten = ref("");
  const newRuleRegex = ref("");
  const newRuleReplacement = ref("");
  const newRulePriority = ref(0);
  const previewTestInput = ref("");

  // ── Syscall Interception State ──
  const disabledEventTypes = ref<Set<number>>(new Set());

  // ── Kernel / cgroup enforcement state ──
  const cgroupSandboxStatus = ref({
    available: false,
    attached: false,
    cgroupPath: "",
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
    statsError: "",
    error: "",
  });
  const cgroupSandboxLoading = ref(false);
  const cgroupTargetID = ref("");
  const cgroupTargetPID = ref<number | null>(null);
  const cgroupTargetIP = ref("");
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
    statsError: "",
    error: "",
  });
  const lsmEnforcerLoading = ref(false);
  const lsmExecPath = ref("/usr/bin/nc");
  const lsmExecName = ref("nc");
  const lsmFileName = ref("id_rsa");

  // ── External Rule Import State ──
  const fetchedExternalRules = ref<WrapperRule[]>([]);
  const fetchSourceLoading = ref<string | null>(null);
  const importingExternalRules = ref(false);

  // ── Wrapper Rules CRUD ──
  const fetchRules = async () => {
    try {
      const res = await axios.get("/config/rules");
      wrapperRules.value = res.data;
    } catch (_) {}
  };

  const postRule = async (rule: WrapperRule) => {
    await axios.post("/config/rules", rule);
  };

  const buildManualRulePayload = (): WrapperRule => ({
    comm: newRuleComm.value,
    action: newRuleAction.value,
    rewritten_cmd:
      newRuleAction.value === "REWRITE" && !newRuleRegex.value
        ? newRuleRewritten.value.split(" ").filter((s) => s)
        : [],
    regex: newRuleRegex.value,
    replacement: newRuleReplacement.value,
    priority: newRulePriority.value,
  });

  const resetRuleForm = () => {
    newRuleComm.value = "";
    newRuleRewritten.value = "";
    newRuleRegex.value = "";
    newRuleReplacement.value = "";
    newRulePriority.value = 0;
    previewTestInput.value = "";
  };

  const saveRule = async () => {
    if (!newRuleComm.value) return;
    try {
      await postRule(buildManualRulePayload());
      message.success("Rule saved");
      resetRuleForm();
      await fetchRules();
    } catch (_) {
      message.error("Failed to save rule");
    }
  };

  const deleteRule = async (comm: string) => {
    try {
      await axios.delete(`/config/rules/${comm}`);
      message.success("Rule deleted");
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
      message.info(
        `已加载 ${fetchedExternalRules.value.length} 条 OWASP 预设规则（本地提供）`,
      );
      return;
    }
    fetchSourceLoading.value = source.id;
    try {
      const res = await axios.get(source.url, { timeout: 15000 });
      if (source.id === "secure-code-warrior") {
        const rules = res.data?.rules || res.data || [];
        fetchedExternalRules.value = (Array.isArray(rules) ? rules : [])
          .map((r: any) => ({
            comm: r.command || r.comm || r.name || "",
            action:
              r.severity === "critical" || r.action === "BLOCK"
                ? "BLOCK"
                : "ALERT",
            rewritten_cmd: [],
            priority: r.priority ?? 180,
          }))
          .filter((r: WrapperRule) => r.comm);
      } else if (source.id === "claude-code-safety-net") {
        const rules = res.data?.rules || res.data || [];
        fetchedExternalRules.value = (Array.isArray(rules) ? rules : [])
          .map((r: any) => ({
            comm: r.command || r.comm || r.name || "",
            action: r.action || "ALERT",
            rewritten_cmd: [],
            priority: r.priority ?? 180,
          }))
          .filter((r: WrapperRule) => r.comm);
      } else {
        fetchedExternalRules.value = [];
      }
      message.success(
        `从 ${source.name} 获取到 ${fetchedExternalRules.value.length} 条规则`,
      );
    } catch (e: any) {
      message.error(`获取 ${source.name} 失败：${e.message || "网络错误"}`);
      fetchedExternalRules.value = [];
    } finally {
      fetchSourceLoading.value = null;
    }
  };

  const importAllFetchedRules = async () => {
    if (!fetchedExternalRules.value.length) {
      message.warning("没有可导入的规则，请先获取外部来源");
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
      message.warning(
        `外部规则导入完成：成功 ${success} 条，失败 ${failed} 条`,
      );
    } else {
      message.success(`外部规则导入完成：${success} 条全部写入`);
    }
    fetchedExternalRules.value = [];
    importingExternalRules.value = false;
  };

  // ── Syscall Toggles ──
  const fetchDisabledEventTypes = async () => {
    try {
      const res = await axios.get("/config/event-types");
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
      const res = await axios.get("/sandbox/cgroup/status");
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
      message.error(
        `加载 cgroup sandbox 状态失败：${e.response?.data?.error || e.message || "unknown error"}`,
      );
    } finally {
      cgroupSandboxLoading.value = false;
    }
  };

  const postCgroupSandboxAction = async (
    path: string,
    payload: Record<string, unknown>,
    successText: CgroupSandboxSuccessText,
  ) => {
    cgroupSandboxLoading.value = true;
    try {
      const res = await axios.post<CgroupSandboxActionResponse>(path, payload);
      const successMessage =
        typeof successText === "function"
          ? successText(res.data || {})
          : successText;
      message.success(successMessage);
      await fetchCgroupSandboxStatus();
    } catch (e: any) {
      message.error(
        e.response?.data?.error ||
          "cgroup sandbox 操作失败；请确认 /config/runtime 已启用 policy management",
      );
    } finally {
      cgroupSandboxLoading.value = false;
    }
  };

  const blockCgroupID = async () => {
    const cgroupId = cgroupTargetID.value.trim();
    if (!/^[1-9]\d*$/.test(cgroupId)) {
      message.warning("请输入有效的 cgroup id");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/block-cgroup",
      { cgroupId },
      `已阻断 cgroup ${cgroupId} 的出站连接`,
    );
  };

  const unblockCgroupID = async () => {
    const cgroupId = cgroupTargetID.value.trim();
    if (!/^[1-9]\d*$/.test(cgroupId)) {
      message.warning("请输入有效的 cgroup id");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/unblock-cgroup",
      { cgroupId },
      `已解除 cgroup ${cgroupId} 的出站阻断`,
    );
  };

  const blockCgroupPID = async () => {
    if (!cgroupTargetPID.value || cgroupTargetPID.value <= 0) {
      message.warning("请输入有效的 PID");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/block-pid",
      { pid: cgroupTargetPID.value },
      `已阻断 PID ${cgroupTargetPID.value} 所在 cgroup 的出站连接`,
    );
  };

  const unblockCgroupPID = async () => {
    if (!cgroupTargetPID.value || cgroupTargetPID.value <= 0) {
      message.warning("请输入有效的 PID");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/unblock-pid",
      { pid: cgroupTargetPID.value },
      `已解除 PID ${cgroupTargetPID.value} 所在 cgroup 的出站阻断`,
    );
  };

  const blockCgroupIP = async () => {
    const ip = cgroupTargetIP.value.trim();
    if (!ip) {
      message.warning("请输入 IPv4、IPv6 或 IPv4-mapped IPv6 地址");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/block-ip",
      { ip },
      (data) => `已在内核层阻断 ${data.ip || ip}`,
    );
  };

  const unblockCgroupIP = async () => {
    const ip = cgroupTargetIP.value.trim();
    if (!ip) {
      message.warning("请输入 IPv4、IPv6 或 IPv4-mapped IPv6 地址");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/unblock-ip",
      { ip },
      (data) => `已解除 ${data.ip || ip} 的内核层阻断`,
    );
  };

  const blockCgroupPort = async () => {
    if (!cgroupTargetPort.value) {
      message.warning("请输入端口");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/block-port",
      { port: cgroupTargetPort.value },
      `已在内核层阻断端口 ${cgroupTargetPort.value}`,
    );
  };

  const unblockCgroupPort = async () => {
    if (!cgroupTargetPort.value) {
      message.warning("请输入端口");
      return;
    }
    await postCgroupSandboxAction(
      "/sandbox/cgroup/unblock-port",
      { port: cgroupTargetPort.value },
      `已解除端口 ${cgroupTargetPort.value} 的内核层阻断`,
    );
  };

  // ── Kernel / BPF LSM enforcement ──
  const fetchLsmEnforcerStatus = async () => {
    lsmEnforcerLoading.value = true;
    try {
      const res = await axios.get("/sandbox/lsm/status");
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
      message.error(
        `加载 BPF LSM 状态失败：${e.response?.data?.error || e.message || "unknown error"}`,
      );
    } finally {
      lsmEnforcerLoading.value = false;
    }
  };

  const postLsmEnforcerAction = async (
    path: string,
    payload: Record<string, unknown>,
    successText: string,
  ) => {
    lsmEnforcerLoading.value = true;
    try {
      await axios.post(path, payload);
      message.success(successText);
      await fetchLsmEnforcerStatus();
    } catch (e: any) {
      message.error(
        e.response?.data?.error ||
          "BPF LSM 操作失败；请确认内核启用了 BPF LSM 且 /config/runtime 已启用 policy management",
      );
    } finally {
      lsmEnforcerLoading.value = false;
    }
  };

  const blockLsmExecPath = async () => {
    const path = lsmExecPath.value.trim();
    if (!path) {
      message.warning("请输入要拦截的执行路径");
      return;
    }
    await postLsmEnforcerAction(
      "/sandbox/lsm/block-exec-path",
      { path },
      `已在 BPF LSM 阻断执行：${path}`,
    );
  };

  const unblockLsmExecPath = async (path = lsmExecPath.value.trim()) => {
    if (!path) {
      message.warning("请输入要解除的执行路径");
      return;
    }
    await postLsmEnforcerAction(
      "/sandbox/lsm/unblock-exec-path",
      { path },
      `已解除 BPF LSM 执行阻断：${path}`,
    );
  };

  const blockLsmExecName = async () => {
    const name = lsmExecName.value.trim();
    if (!name) {
      message.warning("请输入要拦截的可执行文件名");
      return;
    }
    await postLsmEnforcerAction(
      "/sandbox/lsm/block-exec-name",
      { name },
      `已在 BPF LSM 阻断可执行文件名：${name}`,
    );
  };

  const unblockLsmExecName = async (name = lsmExecName.value.trim()) => {
    if (!name) {
      message.warning("请输入要解除的可执行文件名");
      return;
    }
    await postLsmEnforcerAction(
      "/sandbox/lsm/unblock-exec-name",
      { name },
      `已解除 BPF LSM 可执行文件名阻断：${name}`,
    );
  };

  const blockLsmFileName = async () => {
    const name = lsmFileName.value.trim();
    if (!name) {
      message.warning("请输入要拦截的文件或目录 basename");
      return;
    }
    await postLsmEnforcerAction(
      "/sandbox/lsm/block-file-name",
      { name },
      `已在 BPF LSM 阻断打开/读写/mmap/mprotect/setattr/创建/link/symlink/删除/mkdir/rmdir/mknod/rename basename：${name}`,
    );
  };

  const unblockLsmFileName = async (name = lsmFileName.value.trim()) => {
    if (!name) {
      message.warning("请输入要解除的文件或目录 basename");
      return;
    }
    await postLsmEnforcerAction(
      "/sandbox/lsm/unblock-file-name",
      { name },
      `已解除 BPF LSM 打开/读写/mmap/mprotect/setattr/创建/link/symlink/删除/mkdir/rmdir/mknod/rename basename 阻断：${name}`,
    );
  };

  // ── Regex Preview ──
  const regexPreviewResult = computed(() => {
    if (!newRuleRegex.value || !previewTestInput.value) return "";
    try {
      const re = new RegExp(newRuleRegex.value);
      return previewTestInput.value.replace(re, newRuleReplacement.value);
    } catch (_) {
      return "Invalid Regex";
    }
  });

  return {
    wrapperRules,
    newRuleComm,
    newRuleAction,
    newRuleRewritten,
    newRuleRegex,
    newRuleReplacement,
    newRulePriority,
    previewTestInput,
    disabledEventTypes,
    cgroupSandboxStatus,
    cgroupSandboxLoading,
    cgroupTargetID,
    cgroupTargetPID,
    cgroupTargetIP,
    cgroupTargetPort,
    lsmEnforcerStatus,
    lsmEnforcerLoading,
    lsmExecPath,
    lsmExecName,
    lsmFileName,
    fetchedExternalRules,
    fetchSourceLoading,
    importingExternalRules,
    fetchRules,
    postRule,
    saveRule,
    deleteRule,
    buildManualRulePayload,
    resetRuleForm,
    addQuickRulePreset,
    addAllQuickRulePresets,
    fetchExternalRules,
    importAllFetchedRules,
    fetchDisabledEventTypes,
    toggleEventType,
    fetchCgroupSandboxStatus,
    blockCgroupID,
    unblockCgroupID,
    blockCgroupPID,
    unblockCgroupPID,
    blockCgroupIP,
    unblockCgroupIP,
    blockCgroupPort,
    unblockCgroupPort,
    fetchLsmEnforcerStatus,
    blockLsmExecPath,
    unblockLsmExecPath,
    blockLsmExecName,
    unblockLsmExecName,
    blockLsmFileName,
    unblockLsmFileName,
    regexPreviewResult,
  };
}
