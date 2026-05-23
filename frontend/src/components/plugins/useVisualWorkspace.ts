import { ref, computed, watch, nextTick } from "vue";
import { message } from "ant-design-vue";
import type {
  VisualTrigger,
  VisualAction,
  VisualMapMode,
  VisualMapKey,
  VisualLogicGroup,
  VisualLogicNode,
  VisualCondition,
  VisualWorkspaceSnapshot,
  VisualNodeLayout,
  VisualWireStates,
  VisualHiddenNodeStates,
  VisualFlowNodeId,
} from "./types";
import { triggerOptions } from "./constants";
import {
  countConditions,
  getTreeDepth,
  assertValidConditionTree,
  isVisualConditionField,
} from "./validation";

export function useVisualWorkspace() {
  const trigger = ref<VisualTrigger>("process");

  const logicRoot = ref<VisualLogicGroup>({
    id: "root",
    type: "AND",
    children: [
      {
        id: "cond-init",
        type: "CONDITION",
        field: "comm",
        operator: "==",
        value: "nc",
      },
    ],
  });

  const action = ref<VisualAction>("BLOCK");

  // Stateful Map configurations
  const mapMode = ref<VisualMapMode>("NONE");
  const mapKey = ref<VisualMapKey>("pid");
  const mapLimit = ref<number>(10);

  // AI Copilot
  const aiPrompt = ref("");

  // Pseudo-code Editor
  const pseudoCode = ref("");
  const usePseudoCode = ref(false);

  // Manifest metadata
  const pluginId = ref("visual-plugin-custom-block");
  const pluginName = ref("可视化流插件(custom-block)");
  const description = ref(
    "利用图形化流式积木拼装自动生成的内核级 eBPF 拦截器。"
  );

  const isCompiled = ref(false);
  const autosaveLabel = ref("本地草稿未加载");

  // History Undo/Redo
  const undoStack = ref<VisualWorkspaceSnapshot[]>([]);
  const redoStack = ref<VisualWorkspaceSnapshot[]>([]);
  const lastHistoryJson = ref("");
  const isHistoryApplying = ref(false);
  const maxHistoryDepth = 40;

  // Node layout & wire states defaults
  const createDefaultNodeLayout = (): VisualNodeLayout => ({
    trigger: { x: 24, y: 38 },
    condition: { x: 196, y: 38 },
    map: { x: 368, y: 38 },
    action: { x: 540, y: 38 },
    code: { x: 368, y: 176 },
    compile: { x: 540, y: 176 },
  });

  const createDefaultWireStates = (): Record<string, boolean> => ({
    "trigger-condition": true,
    "condition-map": true,
    "map-action": true,
    "condition-code": true,
    "map-code": true,
    "action-compile": true,
    "code-compile": true,
  });

  const createDefaultHiddenNodes = (): VisualHiddenNodeStates => ({});

  const nodeLayout = ref<VisualNodeLayout>(createDefaultNodeLayout());
  const wireStates = ref<VisualWireStates>(createDefaultWireStates());
  const hiddenFlowNodes = ref<VisualHiddenNodeStates>(
    createDefaultHiddenNodes()
  );

  const activeFlowNode = ref<VisualFlowNodeId>("trigger");
  const designerSubtab = ref<"dify" | "map" | "nlp" | "source">("dify");

  const workspaceStorageKey = "agent-ebpf-filter.visual-ebpf.workspace.v1";

  // Helpers
  const canUseLocalStorage = () =>
    typeof window !== "undefined" && typeof window.localStorage !== "undefined";

  const getTimeLabel = () => new Date().toLocaleTimeString();

  const cloneLogicRoot = (root: VisualLogicGroup): VisualLogicGroup =>
    JSON.parse(JSON.stringify(root)) as VisualLogicGroup;

  const createWorkspaceSnapshot = (): VisualWorkspaceSnapshot => ({
    version: 1,
    trigger: trigger.value,
    action: action.value,
    conditions: cloneLogicRoot(logicRoot.value),
    mapMode: mapMode.value,
    mapKey: mapKey.value,
    mapLimit: mapLimit.value,
    nodeLayout: { ...nodeLayout.value },
    wireStates: { ...wireStates.value },
    hiddenNodes: { ...hiddenFlowNodes.value },
    pluginId: pluginId.value,
    pluginName: pluginName.value,
    description: description.value,
    pseudoCode: pseudoCode.value,
    usePseudoCode: usePseudoCode.value,
  });

  const cloneWorkspaceSnapshot = (
    snapshot: VisualWorkspaceSnapshot
  ): VisualWorkspaceSnapshot =>
    JSON.parse(JSON.stringify(snapshot)) as VisualWorkspaceSnapshot;

  const serializeWorkspaceSnapshot = (snapshot: VisualWorkspaceSnapshot) =>
    JSON.stringify(snapshot);

  const applyWorkspaceSnapshot = (snapshot: VisualWorkspaceSnapshot) => {
    const validTrigger = triggerOptions.some(
      (item) => item.value === snapshot.trigger
    );
    if (!validTrigger) throw new Error(`不支持的挂载点: ${snapshot.trigger}`);
    if (!snapshot.conditions || !Array.isArray(snapshot.conditions.children)) {
      throw new Error("conditions 必须是包含 children 的逻辑根节点");
    }
    assertValidConditionTree(snapshot.conditions);
    const conditionTotal = countConditions(snapshot.conditions);
    if (conditionTotal > 8) {
      throw new Error(
        `条件数量 ${conditionTotal} 超过 eBPF Verifier 友好上限 8`
      );
    }
    const visualMapModeSet = new Set<VisualMapMode>([
      "NONE",
      "COUNTER",
      "BLOCKLIST",
    ]);
    if (!visualMapModeSet.has(snapshot.mapMode)) {
      throw new Error(`不支持的 Map 模式: ${String(snapshot.mapMode)}`);
    }
    const visualMapKeySet = new Set<VisualMapKey>(["uid", "pid", "comm"]);
    if (!visualMapKeySet.has(snapshot.mapKey)) {
      throw new Error(`不支持的 Map Key: ${String(snapshot.mapKey)}`);
    }

    trigger.value = snapshot.trigger;
    action.value =
      snapshot.trigger === "unlink" && snapshot.action === "BLOCK"
        ? "ALERT"
        : snapshot.action;
    logicRoot.value = cloneLogicRoot(snapshot.conditions);
    mapMode.value = snapshot.mapMode || "NONE";
    mapKey.value = snapshot.mapKey || "pid";
    mapLimit.value = Number(snapshot.mapLimit) || 10;
    if (snapshot.pluginId) pluginId.value = snapshot.pluginId;
    if (snapshot.pluginName) pluginName.value = snapshot.pluginName;
    if (snapshot.description) description.value = snapshot.description;
    if (snapshot.pseudoCode !== undefined)
      pseudoCode.value = snapshot.pseudoCode;
    if (snapshot.usePseudoCode !== undefined)
      usePseudoCode.value = snapshot.usePseudoCode;
    nodeLayout.value = snapshot.nodeLayout
      ? { ...createDefaultNodeLayout(), ...snapshot.nodeLayout }
      : createDefaultNodeLayout();
    wireStates.value = snapshot.wireStates
      ? { ...createDefaultWireStates(), ...snapshot.wireStates }
      : createDefaultWireStates();
    hiddenFlowNodes.value = snapshot.hiddenNodes
      ? { ...createDefaultHiddenNodes(), ...snapshot.hiddenNodes }
      : createDefaultHiddenNodes();
  };

  // Draft LocalStorage
  const saveWorkspaceDraft = (silent = false) => {
    if (!canUseLocalStorage()) {
      autosaveLabel.value = "当前环境不支持 localStorage 草稿";
      return;
    }
    window.localStorage.setItem(
      workspaceStorageKey,
      JSON.stringify(createWorkspaceSnapshot())
    );
    autosaveLabel.value = `草稿已保存 ${getTimeLabel()}`;
    if (!silent) message.success("低代码积木草稿已保存到浏览器本地");
  };

  const clearWorkspaceDraft = () => {
    if (!canUseLocalStorage()) return;
    window.localStorage.removeItem(workspaceStorageKey);
    autosaveLabel.value = "草稿已清除；继续编辑会自动重新保存";
    message.success("已清除浏览器本地积木草稿");
  };

  const restoreWorkspaceDraft = () => {
    if (!canUseLocalStorage()) return;
    const raw = window.localStorage.getItem(workspaceStorageKey);
    if (!raw) {
      autosaveLabel.value = "尚无浏览器本地草稿";
      return;
    }
    try {
      applyWorkspaceSnapshot(JSON.parse(raw) as VisualWorkspaceSnapshot);
      autosaveLabel.value = `已恢复本地草稿 ${getTimeLabel()}`;
      message.info("已恢复上次未完成的低代码积木草稿");
    } catch (err: any) {
      autosaveLabel.value = "本地草稿损坏，已忽略";
      console.warn("Failed to restore visual eBPF workspace draft:", err);
    }
  };

  // History stack
  const syncHistoryBaseline = () => {
    lastHistoryJson.value = serializeWorkspaceSnapshot(
      createWorkspaceSnapshot()
    );
  };

  const recordWorkspaceHistory = () => {
    if (isHistoryApplying.value) return;
    const nextSnapshot = createWorkspaceSnapshot();
    const nextJson = serializeWorkspaceSnapshot(nextSnapshot);
    if (!lastHistoryJson.value) {
      lastHistoryJson.value = nextJson;
      return;
    }
    if (nextJson === lastHistoryJson.value) return;
    undoStack.value.push(
      JSON.parse(lastHistoryJson.value) as VisualWorkspaceSnapshot
    );
    if (undoStack.value.length > maxHistoryDepth) {
      undoStack.value.shift();
    }
    redoStack.value = [];
    lastHistoryJson.value = nextJson;
  };

  const applyHistorySnapshot = async (snapshot: VisualWorkspaceSnapshot) => {
    isHistoryApplying.value = true;
    applyWorkspaceSnapshot(cloneWorkspaceSnapshot(snapshot));
    lastHistoryJson.value = serializeWorkspaceSnapshot(
      createWorkspaceSnapshot()
    );
    await nextTick();
    isHistoryApplying.value = false;
    saveWorkspaceDraft(true);
  };

  const undoWorkspace = async () => {
    const previous = undoStack.value.pop();
    if (!previous) {
      message.info("当前没有可撤销的积木历史");
      return;
    }
    redoStack.value.push(cloneWorkspaceSnapshot(createWorkspaceSnapshot()));
    await applyHistorySnapshot(previous);
    message.success("已撤销上一步积木编辑");
  };

  const redoWorkspace = async () => {
    const next = redoStack.value.pop();
    if (!next) {
      message.info("当前没有可重做的积木历史");
      return;
    }
    undoStack.value.push(cloneWorkspaceSnapshot(createWorkspaceSnapshot()));
    await applyHistorySnapshot(next);
    message.success("已重做积木编辑");
  };

  // Node helpers
  const countConditionsLocal = computed(() => countConditions(logicRoot.value));
  const treeDepthLocal = computed(() => getTreeDepth(logicRoot.value));

  const findNodeAndMutate = (
    root: VisualLogicGroup,
    targetId: string,
    mutateFn: (parent: VisualLogicGroup, index: number) => void
  ): boolean => {
    if (root.id === targetId) return false;
    for (let i = 0; i < root.children.length; i++) {
      const child = root.children[i];
      if (child.id === targetId) {
        mutateFn(root, i);
        return true;
      }
      if (child.type === "AND" || child.type === "OR") {
        const found = findNodeAndMutate(child, targetId, mutateFn);
        if (found) return true;
      }
    }
    return false;
  };

  const findNodeById = (
    root: VisualLogicNode,
    targetId: string
  ): VisualLogicNode | null => {
    if (root.id === targetId) return root;
    if (root.type === "AND" || root.type === "OR") {
      if (root.children) {
        for (const child of root.children) {
          const found = findNodeById(child, targetId);
          if (found) return found;
        }
      }
    }
    return null;
  };

  const onDeleteNode = (id: string) => {
    if (id === "root") return;
    findNodeAndMutate(logicRoot.value, id, (parent, idx) => {
      parent.children.splice(idx, 1);
    });
  };

  const onAddRule = (groupId: string, field?: string) => {
    const currentCount = countConditions(logicRoot.value);
    if (currentCount >= 8) {
      message.warning(
        "为了防止 eBPF Verifier 复杂度过高而加载失败，图形化条件最多限制为 8 个"
      );
      return;
    }
    const targetGroup = findNodeById(logicRoot.value, groupId);
    if (
      targetGroup &&
      (targetGroup.type === "AND" || targetGroup.type === "OR")
    ) {
      const id = `cond-${Math.random().toString(36).substr(2, 9)}`;
      const fieldValue = isVisualConditionField(field) ? field : "comm";
      targetGroup.children.push({
        id,
        type: "CONDITION",
        field: fieldValue,
        operator: "==",
        value: "",
      });
    }
  };

  const onAddGroup = (groupId: string, type: "AND" | "OR") => {
    const targetGroup = findNodeById(logicRoot.value, groupId);
    if (
      targetGroup &&
      (targetGroup.type === "AND" || targetGroup.type === "OR")
    ) {
      const id = `group-${Math.random().toString(36).substr(2, 9)}`;
      targetGroup.children.push({
        id,
        type,
        children: [],
      });
    }
  };

  const onUpdateRule = (ruleId: string, updated: Partial<VisualCondition>) => {
    const targetNode = findNodeById(logicRoot.value, ruleId);
    if (targetNode && targetNode.type === "CONDITION") {
      Object.assign(targetNode, updated);
    }
  };

  const onUpdateGroupType = (groupId: string, type: "AND" | "OR") => {
    const targetNode = findNodeById(logicRoot.value, groupId);
    if (targetNode && (targetNode.type === "AND" || targetNode.type === "OR")) {
      targetNode.type = type;
    }
  };

  // Sync state watch
  watch(
    [trigger, logicRoot, action, mapMode, mapKey, mapLimit],
    async () => {
      if (isHistoryApplying.value) {
        isCompiled.value = false;
        return;
      }
      // Sanitize conditions recursively
      const sanitizeNode = (node: VisualLogicNode) => {
        if (node.type === "CONDITION") {
          if (trigger.value !== "socket_connect") {
            if (node.field === "port" || node.field === "ipv4") {
              node.field = "comm";
            }
          }
          if (trigger.value === "unlink") {
            if (node.field === "basename") {
              node.field = "comm";
            }
          }
        } else if (node.children) {
          node.children.forEach(sanitizeNode);
        }
      };
      sanitizeNode(logicRoot.value);

      // Extract first condition value to make descriptive id
      const leaves: VisualCondition[] = [];
      const findLeaves = (n: VisualLogicNode) => {
        if (n.type === "CONDITION") leaves.push(n);
        else if (n.children) n.children.forEach(findLeaves);
      };
      findLeaves(logicRoot.value);

      const firstVal = leaves[0]?.value || "custom";
      let prefix = `visual-block-${trigger.value}-${firstVal.replace(
        /[^a-z0-9]/g,
        "-"
      )}`.toLowerCase();

      // Ensure the generated plugin ID strictly complies with: 3-64 chars, lowercase, alphanumeric or hyphen, starts with alpha/num
      prefix = prefix.replace(/^[^a-z0-9]+/g, ""); // Remove invalid leading characters
      prefix = prefix.replace(/[^a-z0-9-]/g, "-"); // Replace other invalid characters with hyphens
      prefix = prefix.replace(/-+/g, "-"); // Deduplicate hyphens
      prefix = prefix.substring(0, 64); // Cap at 64 characters
      if (prefix.length < 3) {
        prefix = `visual-block-${prefix || "plugin"}`;
      }

      pluginId.value = prefix;
      pluginName.value = `积木插件(${trigger.value}-${firstVal})`;
      description.value = `由图形化积木拼装而成的内核 eBPF 过滤审计插件。入口: ${
        trigger.value
      }，Map状态: ${mapMode.value}，嵌套层数: ${getTreeDepth(
        logicRoot.value
      )}，动作: ${action.value}。`;
      isCompiled.value = false;

      // Automatically generate pseudo-code if not using manual pseudo-code compilation mode
      if (!usePseudoCode.value) {
        const { snapshotToPseudoCode } = await import("./transpiler-ts");
        pseudoCode.value = snapshotToPseudoCode(createWorkspaceSnapshot());
      }
    },
    { deep: true, immediate: true }
  );

  watch(
    [
      trigger,
      logicRoot,
      action,
      mapMode,
      mapKey,
      mapLimit,
      nodeLayout,
      wireStates,
      hiddenFlowNodes,
      pluginId,
      pluginName,
      description,
      pseudoCode,
      usePseudoCode,
    ],
    () => {
      saveWorkspaceDraft(true);
      recordWorkspaceHistory();
    },
    { deep: true }
  );

  return {
    trigger,
    logicRoot,
    action,
    mapMode,
    mapKey,
    mapLimit,
    aiPrompt,
    pseudoCode,
    usePseudoCode,
    pluginId,
    pluginName,
    description,
    isCompiled,
    autosaveLabel,
    undoStack,
    redoStack,
    nodeLayout,
    wireStates,
    hiddenFlowNodes,
    activeFlowNode,
    designerSubtab,
    countConditions: countConditionsLocal,
    treeDepth: treeDepthLocal,
    onDeleteNode,
    onAddRule,
    onAddGroup,
    onUpdateRule,
    onUpdateGroupType,
    createWorkspaceSnapshot,
    applyWorkspaceSnapshot,
    saveWorkspaceDraft,
    clearWorkspaceDraft,
    restoreWorkspaceDraft,
    syncHistoryBaseline,
    undoWorkspace,
    redoWorkspace,
  };
}
