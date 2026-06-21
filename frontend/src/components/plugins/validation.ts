import type {
  VisualWorkspaceSnapshot,
  VisualValidationIssue,
  VisualConditionField,
  VisualLogicNode,
  VisualCondition,
} from "./types";

export const isValidIPv4 = (value: string): boolean => {
  const parts = value.split(".");
  return (
    parts.length === 4 &&
    parts.every((part) => {
      if (!/^\d+$/.test(part)) return false;
      const num = Number(part);
      return num >= 0 && num <= 255;
    })
  );
};

const visualFieldSet = new Set<VisualConditionField>([
  "comm",
  "pid",
  "uid",
  "basename",
  "port",
  "ipv4",
  "gid",
  "ppid",
  "loginuid",
]);

export const isVisualConditionField = (
  value: unknown,
): value is VisualConditionField =>
  typeof value === "string" &&
  visualFieldSet.has(value as VisualConditionField);

export const countConditions = (node: VisualLogicNode): number => {
  if (node.type === "CONDITION") return 1;
  if (!node.children) return 0;
  return node.children.reduce((sum, child) => sum + countConditions(child), 0);
};

export const getTreeDepth = (node: VisualLogicNode): number => {
  if (node.type === "CONDITION") return 1;
  if (!node.children || node.children.length === 0) return 1;
  return 1 + Math.max(...node.children.map(getTreeDepth));
};

export const assertValidConditionTree = (node: VisualLogicNode) => {
  if (node.type === "CONDITION") {
    if (!isVisualConditionField(node.field)) {
      throw new Error(`不支持的条件字段：${String(node.field)}`);
    }
    return;
  }
  if (node.type !== "AND" && node.type !== "OR") {
    throw new Error("逻辑分组必须是 AND 或 OR");
  }
  if (!Array.isArray(node.children)) {
    throw new Error("逻辑分组 children 必须 be 数组");
  }
  node.children.forEach(assertValidConditionTree);
};

export const validateWorkspace = (
  snapshot: VisualWorkspaceSnapshot,
  flowNodeDetails: Record<string, { label: string }>,
  visualWireLabels: Record<string, string>,
  visualWireEndpoints: Record<string, { from: string; to: string }>,
  visualFlowNodeIds: string[],
  visualWireIds: string[],
): VisualValidationIssue[] => {
  const issues: VisualValidationIssue[] = [];

  const {
    pluginId = "",
    hiddenNodes = {},
    wireStates = {},
    conditions,
    trigger,
    action,
    mapMode,
    mapLimit,
  } = snapshot;

  // 1. Plugin ID validation
  if (!/^[a-z0-9][a-z0-9-]{2,63}$/.test(pluginId.trim())) {
    issues.push({
      id: "plugin-id",
      severity: "error",
      title: "插件 ID 不合法",
      detail: "请使用 3-64 位小写字母、数字或中划线，且以字母/数字开头。",
    });
  }

  // 2. Deleted nodes validation
  const deletedNodes = visualFlowNodeIds.filter(
    (id) => (hiddenNodes as Record<string, boolean>)[id],
  );
  if (deletedNodes.length > 0) {
    issues.push({
      id: "flow-node-deleted",
      severity: "error",
      title: "低代码流程节点已删除",
      detail: `请从左侧节点类型库恢复：${deletedNodes
        .map((id) => flowNodeDetails[id]?.label || id)
        .join("、")}。`,
    });
  }

  // 3. Disconnected wires validation
  const disconnectedWires = visualWireIds.filter((id) => {
    const endpoint = visualWireEndpoints[id];
    if (!endpoint) return false;
    if (
      (hiddenNodes as Record<string, boolean>)[endpoint.from] ||
      (hiddenNodes as Record<string, boolean>)[endpoint.to]
    ) {
      return false;
    }
    return !(wireStates as Record<string, boolean>)[id];
  });
  if (disconnectedWires.length > 0) {
    issues.push({
      id: "wire-flow-disconnected",
      severity: "error",
      title: "低代码流程线缆未闭合",
      detail: `请在画布中重新连接：${disconnectedWires
        .map((id) => visualWireLabels[id] || id)
        .join("、")}。`,
    });
  }

  // 4. Condition Count & Depth validation
  const conditionCount = countConditions(conditions);
  if (conditionCount === 0) {
    issues.push({
      id: "no-conditions",
      severity: "error",
      title: "条件积木为空",
      detail: "至少需要一个 CONDITION 积木，否则生成的规则没有明确匹配边界。",
    });
  }

  if (conditionCount > 8) {
    issues.push({
      id: "condition-limit",
      severity: "error",
      title: "条件积木过多",
      detail: "当前限制为 8 个条件，避免 eBPF Verifier 复杂度过高。",
    });
  }

  const treeDepth = getTreeDepth(conditions);
  if (treeDepth > 5) {
    issues.push({
      id: "tree-depth",
      severity: "warning",
      title: "逻辑嵌套较深",
      detail: "建议把深层 AND/OR 拆成多个插件，便于审计和 verifier 排错。",
    });
  }

  // 5. Unlink block validation
  if (trigger === "unlink" && action === "BLOCK") {
    issues.push({
      id: "unlink-block",
      severity: "error",
      title: "unlink Kprobe 不支持 BLOCK",
      detail:
        "Kprobe 只做观测/信号动作，不能直接改变 LSM 返回值；请改用 ALERT 或 KILL。",
    });
  }

  // 6. Map limit validation
  if (mapMode === "COUNTER" && (!Number.isFinite(mapLimit) || mapLimit < 1)) {
    issues.push({
      id: "map-limit",
      severity: "error",
      title: "COUNTER 阈值无效",
      detail: "计数器模式需要大于 0 的最大命中次数。",
    });
  }

  // 7. Blocklist runtime validation
  if (mapMode === "BLOCKLIST") {
    issues.push({
      id: "blocklist-runtime",
      severity: "warning",
      title: "BLOCKLIST 需要运行时填表",
      detail:
        "当前 UI 只声明 map 和查表逻辑；后续还需要通过 bpftool/API 写入具体 key。",
    });
  }

  // 8. Individual condition checks
  const allConditions: VisualCondition[] = [];
  const collect = (node: VisualLogicNode) => {
    if (node.type === "CONDITION") {
      allConditions.push(node);
      return;
    }
    node.children?.forEach(collect);
  };
  collect(conditions);

  allConditions.forEach((condition, index) => {
    const label = `条件 #${index + 1}`;
    const value = (condition.value || "").trim();

    if (!value) {
      issues.push({
        id: `${condition.id}-empty`,
        severity: "error",
        title: `${label} 缺少匹配值`,
        detail: "空值会退化为无边界匹配，已阻止编译。",
      });
      return;
    }

    if (/"|\\|\r|\n/.test(value)) {
      issues.push({
        id: `${condition.id}-unsafe`,
        severity: "error",
        title: `${label} 包含不安全 C 字符`,
        detail:
          "当前可视化生成器暂不接受引号、反斜杠或换行，请改用纯文本匹配值。",
      });
    }

    if (
      trigger !== "socket_connect" &&
      (condition.field === "port" || condition.field === "ipv4")
    ) {
      issues.push({
        id: `${condition.id}-socket-field`,
        severity: "error",
        title: `${label} 字段不适用于当前 Hook`,
        detail: "port / ipv4 仅在 socket_connect 挂载点中有可用上下文。",
      });
    }

    if (trigger === "unlink" && condition.field === "basename") {
      issues.push({
        id: `${condition.id}-unlink-basename`,
        severity: "error",
        title: `${label} 无法读取 unlink basename`,
        detail:
          "当前 unlink 走 kprobe/do_unlinkat，生成器没有安全读取 dentry 名称。",
      });
    }

    if (
      condition.field === "pid" ||
      condition.field === "uid" ||
      condition.field === "gid"
    ) {
      if (!/^\d+$/.test(value)) {
        issues.push({
          id: `${condition.id}-numeric`,
          severity: "error",
          title: `${label} 需要数字值`,
          detail: `${condition.field} 条件只能填写非负整数。`,
        });
      }
    }

    if (condition.field === "port") {
      const port = Number(value);
      if (!/^\d+$/.test(value) || port < 1 || port > 65535) {
        issues.push({
          id: `${condition.id}-port`,
          severity: "error",
          title: `${label} 端口范围无效`,
          detail: "目标端口必须位于 1..65535。",
        });
      }
    }

    if (condition.field === "ipv4" && !isValidIPv4(value)) {
      issues.push({
        id: `${condition.id}-ipv4`,
        severity: "error",
        title: `${label} IPv4 格式无效`,
        detail: "请使用类似 203.0.113.10 的点分十进制地址。",
      });
    }
  });

  if (action === "KILL") {
    issues.push({
      id: "kill-action",
      severity: "info",
      title: "KILL 会发送 SIGKILL",
      detail:
        "生成的程序会调用 bpf_send_signal(9)，建议先用 ALERT 模式演练命中范围。",
    });
  }

  return issues;
};
