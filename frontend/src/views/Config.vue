<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import axios from "axios";
import {
  PlusOutlined,
  TagOutlined,
  AppstoreOutlined,
  FolderOutlined,
  ExportOutlined,
  ImportOutlined,
  SafetyCertificateOutlined,
  ClusterOutlined,
  SwapOutlined,
  StopOutlined,
  AlertOutlined,
  CopyOutlined,
  ReloadOutlined,
  DeleteOutlined,
} from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import {
  getStoredClusterTarget,
  isLocalClusterTarget,
  normalizeClusterTarget,
  setStoredApiToken,
  setStoredClusterTarget,
} from "../utils/requestContext";

interface RuntimeSettings {
  logPersistenceEnabled: boolean;
  logFilePath: string;
  accessToken: string;
  maxEventCount: number;
  maxEventAge: string;
}

interface TrackedItem {
  comm?: string;
  path?: string;
  prefix?: string;
  tag: string;
}

interface WrapperRule {
  comm: string;
  action: string;
  rewritten_cmd: string[];
}

interface ClusterNodeInfo {
  id: string;
  name: string;
  url: string;
  role: "master" | "slave";
  status: string;
  lastSeen: string;
  isLocal: boolean;
  version?: string;
}

interface ClusterStateResponse {
  role: "master" | "slave";
  masterUrl: string;
  nodeUrl: string;
  nodeId: string;
  nodeName: string;
  accountConfigured: boolean;
  passwordConfigured: boolean;
  localNode: ClusterNodeInfo;
}

interface RuntimeConfigResponse {
  runtime: RuntimeSettings;
  mcpEndpoint: string;
  authHeaderName: string;
  bearerAuthHeaderName: string;
  persistedEventLogPath: string;
  persistedEventLogAlive: boolean;
}

const tags = ref<string[]>([]);
const trackedItems = ref<TrackedItem[]>([]);
const trackedPaths = ref<TrackedItem[]>([]);
const trackedPrefixes = ref<TrackedItem[]>([]);
const wrapperRules = ref<Record<string, WrapperRule>>({});
const runtimeSettings = ref<RuntimeSettings>({
  logPersistenceEnabled: false,
  logFilePath: "",
  accessToken: "",
  maxEventCount: 1500,
  maxEventAge: "0",
});
const mcpEndpoint = ref("");
const authHeaderName = ref("X-API-KEY");
const bearerAuthHeaderName = ref("Authorization: Bearer");
const persistedEventLogPath = ref("");
const persistedEventLogAlive = ref(false);
const clusterState = ref<ClusterStateResponse | null>(null);
const clusterNodes = ref<ClusterNodeInfo[]>([]);
const selectedClusterTarget = ref(getStoredClusterTarget());

const newTagName = ref("");
const newCommName = ref("");
const newCommTag = ref("");
const newPathName = ref("");
const newPathTag = ref("");
const newPrefixValue = ref("");
const newPrefixTag = ref("");
const importFileInput = ref<HTMLInputElement | null>(null);

// Wrapper rule state
const newRuleComm = ref("");
const newRuleAction = ref("BLOCK");
const newRuleRewritten = ref("");
const activeTabKey = ref("registry");
const registryTabKey = ref("tags");

const syncApiToken = (token: string) => {
  const normalized = token.trim();
  if (typeof window === "undefined") return;
  if (!normalized) {
    setStoredApiToken("");
    return;
  }
  setStoredApiToken(normalized);
  axios.defaults.headers.common["X-API-KEY"] = normalized;
  axios.defaults.headers.common.Authorization = `Bearer ${normalized}`;
};

const mcpQueryEndpoint = computed(() => {
  if (!mcpEndpoint.value) return "";
  if (!runtimeSettings.value.accessToken.trim()) {
    return `${mcpEndpoint.value}?key=$API_KEY`;
  }
  return `${mcpEndpoint.value}?key=${encodeURIComponent(runtimeSettings.value.accessToken)}`;
});

const mcpQueryEndpointTemplate = computed(() => {
  if (!mcpEndpoint.value) return "";
  return `${mcpEndpoint.value}?key=$API_KEY`;
});

const clusterNodeOptions = computed(() => [
  { label: "Local master", value: "local" },
  ...clusterNodes.value
    .filter((node) => !node.isLocal)
    .map((node) => ({
      label: `${node.name} · ${node.status}`,
      value: node.id,
    })),
]);

const clusterRoleText = computed(() => {
  if (!clusterState.value) return "Unknown";
  return clusterState.value.role === "master" ? "Master" : "Slave";
});

const clusterRoleColor = computed(() =>
  clusterState.value?.role === "slave" ? "orange" : "green",
);

const getClusterRowClass = (record: ClusterNodeInfo) => {
  if (record.id === selectedClusterTarget.value) {
    return "cluster-row-active";
  }
  if (record.isLocal) {
    return "cluster-row-local";
  }
  return "";
};

const updateClusterTargetFromStorage = () => {
  selectedClusterTarget.value = getStoredClusterTarget();
};

const applyClusterTarget = (target: string) => {
  const normalized = normalizeClusterTarget(target);
  setStoredClusterTarget(normalized);
  selectedClusterTarget.value = normalized;
  message.success(
    normalized === "local"
      ? "Routed back to local master"
      : "Cluster target updated",
  );
  window.location.reload();
};

const fetchClusterState = async () => {
  try {
    const res = await axios.get("/cluster/state");
    clusterState.value = res.data as ClusterStateResponse;
  } catch (err) {
    console.error("Failed to fetch cluster state", err);
  }
};

const fetchClusterNodes = async () => {
  try {
    const res = await axios.get("/cluster/nodes");
    clusterNodes.value = (res.data?.nodes || []) as ClusterNodeInfo[];
    if (
      !clusterNodes.value.some(
        (node) => node.id === selectedClusterTarget.value,
      ) &&
      !isLocalClusterTarget(selectedClusterTarget.value)
    ) {
      setStoredClusterTarget("local");
      selectedClusterTarget.value = "local";
    }
  } catch (err) {
    console.error("Failed to fetch cluster nodes", err);
  }
};

const applyRuntimeResponse = (data: RuntimeConfigResponse) => {
  runtimeSettings.value = {
    logPersistenceEnabled: data.runtime.logPersistenceEnabled,
    logFilePath: data.runtime.logFilePath,
    accessToken: data.runtime.accessToken,
    maxEventCount: data.runtime.maxEventCount ?? 1500,
    maxEventAge: data.runtime.maxEventAge ?? "0",
  };
  mcpEndpoint.value = data.mcpEndpoint;
  authHeaderName.value = data.authHeaderName;
  bearerAuthHeaderName.value = data.bearerAuthHeaderName;
  persistedEventLogPath.value = data.persistedEventLogPath;
  persistedEventLogAlive.value = data.persistedEventLogAlive;
  syncApiToken(data.runtime.accessToken);
};

const fetchRuntime = async () => {
  try {
    const res = await axios.get("/config/runtime");
    applyRuntimeResponse(res.data as RuntimeConfigResponse);
  } catch (err) {
    console.error("Failed to fetch runtime config", err);
  }
};

const saveRuntime = async () => {
  try {
    const res = await axios.put("/config/runtime", {
      logPersistenceEnabled: runtimeSettings.value.logPersistenceEnabled,
      logFilePath: runtimeSettings.value.logFilePath,
      maxEventCount: runtimeSettings.value.maxEventCount,
      maxEventAge: runtimeSettings.value.maxEventAge,
    });
    applyRuntimeResponse(res.data as RuntimeConfigResponse);
    message.success("Runtime settings saved");
  } catch (err) {
    message.error("Failed to save runtime settings");
  }
};

const rotateAccessToken = async () => {
  try {
    const res = await axios.post("/config/access-token");
    applyRuntimeResponse(res.data as RuntimeConfigResponse);
    message.success("Access token regenerated");
  } catch (err) {
    message.error("Failed to regenerate access token");
  }
};

const clearInMemoryEvents = async () => {
  try {
    await axios.post("/data/clear-events-memory");
    message.success("In-memory events cleared");
  } catch (err: any) {
    message.error(
      err?.response?.data?.error || "Failed to clear memory events",
    );
  }
};

const clearPersistedLog = async () => {
  try {
    await axios.post("/data/clear-events-persisted");
    message.success("Persisted event log truncated");
  } catch (err: any) {
    message.error(err?.response?.data?.error || "Failed to truncate log");
  }
};

const clearAllEvents = async () => {
  try {
    await axios.post("/data/clear-events");
    message.success("All events cleared");
  } catch (err: any) {
    message.error(err?.response?.data?.error || "Failed to clear events");
  }
};

const copyText = async (text: string, successMessage: string) => {
  const value = text.trim();
  if (!value) {
    message.warning("Nothing to copy");
    return;
  }
  try {
    await navigator.clipboard.writeText(value);
    message.success(successMessage);
  } catch (err) {
    message.error("Failed to copy to clipboard");
  }
};

const fetchTags = async () => {
  try {
    const res = await axios.get("/config/tags");
    tags.value = res.data;
    if (tags.value.length > 0) {
      if (!newCommTag.value) newCommTag.value = tags.value[0];
      if (!newPathTag.value) newPathTag.value = tags.value[0];
    }
  } catch (err) {}
};

const fetchTrackedComms = async () => {
  try {
    const res = await axios.get("/config/comms");
    trackedItems.value = res.data;
  } catch (err) {}
};

const fetchTrackedPaths = async () => {
  try {
    const res = await axios.get("/config/paths");
    trackedPaths.value = res.data;
  } catch (err) {}
};

const fetchTrackedPrefixes = async () => {
  try {
    const res = await axios.get("/config/prefixes");
    trackedPrefixes.value = res.data;
  } catch (err) {}
};

const fetchRules = async () => {
  try {
    const res = await axios.get("/config/rules");
    wrapperRules.value = res.data;
  } catch (err) {}
};

const addTag = async () => {
  if (!newTagName.value) return;
  try {
    await axios.post("/config/tags", { name: newTagName.value });
    message.success(`Tag "${newTagName.value}" created`);
    newTagName.value = "";
    fetchTags();
  } catch (err) {
    message.error("Failed to create tag");
  }
};

const addComm = async () => {
  if (!newCommName.value || !newCommTag.value) return;
  try {
    await axios.post("/config/comms", {
      comm: newCommName.value,
      tag: newCommTag.value,
    });
    message.success(`Added ${newCommName.value}`);
    newCommName.value = "";
    fetchTrackedComms();
  } catch (err) {
    message.error("Failed to add command");
  }
};

const removeComm = async (comm: string) => {
  try {
    await axios.delete(`/config/comms/${comm}`);
    message.success(`Removed ${comm}`);
    fetchTrackedComms();
  } catch (err) {}
};

const addPath = async () => {
  if (!newPathName.value || !newPathTag.value) return;
  try {
    await axios.post("/config/paths", {
      path: newPathName.value,
      tag: newPathTag.value,
    });
    message.success(`Added path ${newPathName.value}`);
    newPathName.value = "";
    fetchTrackedPaths();
  } catch (err) {}
};

const removePath = async (path: string) => {
  try {
    await axios.delete(`/config/paths/${path}`);
    message.success(`Removed path ${path}`);
    fetchTrackedPaths();
  } catch (err) {}
};

const addPrefix = async () => {
  if (!newPrefixValue.value || !newPrefixTag.value) return;
  try {
    await axios.post("/config/prefixes", {
      prefix: newPrefixValue.value,
      tag: newPrefixTag.value,
    });
    message.success(`Added prefix ${newPrefixValue.value}`);
    newPrefixValue.value = "";
    fetchTrackedPrefixes();
  } catch (err) {
    message.error("Failed to add prefix");
  }
};

const removePrefix = async (prefix: string) => {
  try {
    await axios.delete("/config/prefixes", { params: { prefix } });
    message.success(`Removed prefix ${prefix}`);
    fetchTrackedPrefixes();
  } catch (err) {
    message.error("Failed to remove prefix");
  }
};

const saveRule = async () => {
  if (!newRuleComm.value) return;
  try {
    const rule: WrapperRule = {
      comm: newRuleComm.value,
      action: newRuleAction.value,
      rewritten_cmd:
        newRuleAction.value === "REWRITE"
          ? newRuleRewritten.value.split(" ").filter((s) => s)
          : [],
    };
    await axios.post("/config/rules", rule);
    message.success("Rule saved");
    newRuleComm.value = "";
    fetchRules();
  } catch (err) {}
};

const deleteRule = async (comm: string) => {
  try {
    await axios.delete(`/config/rules/${comm}`);
    message.success("Rule deleted");
    fetchRules();
  } catch (err) {}
};

const clearAllConfig = async () => {
  try {
    // Clear Comms
    for (const item of trackedItems.value) {
      if (item.comm) await axios.delete(`/config/comms/${item.comm}`);
    }
    // Clear Paths
    for (const item of trackedPaths.value) {
      if (item.path) await axios.delete(`/config/paths/${item.path}`);
    }
    // Clear Prefixes
    for (const item of trackedPrefixes.value) {
      if (item.prefix)
        await axios.delete("/config/prefixes", {
          params: { prefix: item.prefix },
        });
    }
    // Clear Rules
    for (const comm of Object.keys(wrapperRules.value)) {
      await axios.delete(`/config/rules/${comm}`);
    }
    message.success("All configurations cleared");
    fetchTrackedComms();
    fetchTrackedPaths();
    fetchTrackedPrefixes();
    fetchRules();
  } catch (err) {
    message.error("Failed to clear all configurations");
  }
};

const exportConfig = async () => {
  try {
    const res = await axios.get("/config/export");
    const dataStr =
      "data:text/json;charset=utf-8," +
      encodeURIComponent(JSON.stringify(res.data, null, 2));
    const link = document.createElement("a");
    link.setAttribute("href", dataStr);
    link.setAttribute("download", "agent-ebpf-config.json");
    link.click();
  } catch (err) {}
};

const importConfig = async (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = async (e) => {
    try {
      const config = JSON.parse(e.target?.result as string);
      await axios.post("/config/import", config);
      message.success("Imported");
      fetchTags();
      fetchTrackedComms();
      fetchTrackedPaths();
      fetchTrackedPrefixes();
      fetchRules();
      fetchRuntime();
    } catch (err) {}
  };
  reader.readAsText(file);
};

const groupedTrackedItems = computed(() => {
  const groups: Record<string, string[]> = {};
  trackedItems.value.forEach((item) => {
    if (!groups[item.tag]) groups[item.tag] = [];
    if (item.comm) groups[item.tag].push(item.comm);
  });
  return groups;
});

const groupedTrackedPaths = computed(() => {
  const groups: Record<string, string[]> = {};
  trackedPaths.value.forEach((item) => {
    if (!groups[item.tag]) groups[item.tag] = [];
    if (item.path) groups[item.tag].push(item.path);
  });
  return groups;
});

const groupedTrackedPrefixes = computed(() => {
  const groups: Record<string, string[]> = {};
  trackedPrefixes.value.forEach((item) => {
    if (!groups[item.tag]) groups[item.tag] = [];
    if (item.prefix) groups[item.tag].push(item.prefix);
  });
  return groups;
});

const openImportPicker = () => {
  importFileInput.value?.click();
};

const getCategoryColor = (tag: string) => {
  const colors: Record<string, string> = {
    "AI Agent": "magenta",
    Git: "orange",
    "Build Tool": "cyan",
    "Package Manager": "green",
    Runtime: "blue",
    "System Tool": "geekblue",
    "Network Tool": "purple",
    Security: "red",
    Wrapper: "gold",
  };
  return colors[tag] || "default";
};

onMounted(async () => {
  updateClusterTargetFromStorage();
  await fetchClusterState();
  await fetchClusterNodes();
  await fetchRuntime();
  fetchTags();
  fetchTrackedComms();
  fetchTrackedPaths();
  fetchTrackedPrefixes();
  fetchRules();
});
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%">
    <a-tabs
      v-model:activeKey="activeTabKey"
      type="card"
      size="large"
      :destroyInactiveTabPane="false"
    >
      <!-- Tab 1: eBPF Registry -->
      <a-tab-pane key="registry" tab="eBPF Registry">
        <template #tab>
          <span><TagOutlined /> eBPF Registry</span>
        </template>

        <a-tabs v-model:activeKey="registryTabKey" size="small">
          <!-- Sub-tab 1.1: Tags & Global Management -->
          <a-tab-pane key="tags" tab="Tags & Global">
            <a-row :gutter="[24, 24]">
              <a-col :span="24">
                <a-card title="Global Registry & Actions" size="small">
                  <template #extra>
                    <div style="display: flex; gap: 8px; align-items: center">
                      <input
                        type="file"
                        ref="importFileInput"
                        @change="importConfig"
                        style="display: none"
                        accept=".json"
                      />
                      <a-button size="small" @click="openImportPicker"
                        ><ImportOutlined /> Import</a-button
                      >
                      <a-button size="small" @click="exportConfig"
                        ><ExportOutlined /> Export</a-button
                      >
                      <a-popconfirm
                        title="Are you sure you want to clear all configurations?"
                        @confirm="clearAllConfig"
                      >
                        <a-button size="small" danger>Clear All</a-button>
                      </a-popconfirm>
                      <a-divider type="vertical" />
                      <TagOutlined />
                    </div>
                  </template>
                  <div style="display: flex; flex-direction: column; gap: 16px">
                    <div style="display: flex; gap: 8px; align-items: center">
                      <span style="color: #888; font-size: 13px; width: 80px"
                        >Add Tag:</span
                      >
                      <div style="display: flex; width: 320px">
                        <a-input
                          v-model:value="newTagName"
                          placeholder="New tag name..."
                          @pressEnter="addTag"
                          style="
                            border-top-right-radius: 0;
                            border-bottom-right-radius: 0;
                          "
                        />
                        <a-button
                          type="primary"
                          @click="addTag"
                          style="
                            border-top-left-radius: 0;
                            border-bottom-left-radius: 0;
                          "
                        >
                          <PlusOutlined />
                        </a-button>
                      </div>
                    </div>
                    <div
                      style="display: flex; gap: 8px; align-items: flex-start"
                    >
                      <span
                        style="
                          color: #888;
                          font-size: 13px;
                          width: 80px;
                          margin-top: 4px;
                        "
                        >Registered:</span
                      >
                      <div
                        style="
                          display: flex;
                          flex-wrap: wrap;
                          gap: 8px;
                          flex: 1;
                        "
                      >
                        <a-tag
                          v-for="tag in tags"
                          :key="tag"
                          :color="getCategoryColor(tag)"
                          >{{ tag }}</a-tag
                        >
                      </div>
                    </div>
                  </div>
                </a-card>
              </a-col>
              <a-col :span="24">
                <a-alert
                  type="info"
                  show-icon
                  message="Tags are used to categorize tracked processes, paths, and prefixes. They provide semantic context in the Monitor and Network views."
                />
              </a-col>
            </a-row>
          </a-tab-pane>

          <!-- Sub-tab 1.2: Tracked Binaries -->
          <a-tab-pane key="binaries" tab="Tracked Binaries">
            <a-row :gutter="[24, 24]">
              <a-col :span="24">
                <a-card title="Tracked Executables" size="small">
                  <template #extra><AppstoreOutlined /></template>
                  <div
                    style="
                      margin-bottom: 16px;
                      background: #fafafa;
                      padding: 12px;
                      border-radius: 8px;
                      display: flex;
                      gap: 8px;
                    "
                  >
                    <a-input
                      v-model:value="newCommName"
                      placeholder="Binary name (e.g. curl, git, python)"
                      style="flex: 2"
                    />
                    <a-select
                      v-model:value="newCommTag"
                      style="flex: 1"
                      placeholder="Assign Tag"
                    >
                      <a-select-option
                        v-for="tag in tags"
                        :key="tag"
                        :value="tag"
                        >{{ tag }}</a-select-option
                      >
                    </a-select>
                    <a-button type="primary" @click="addComm"
                      ><PlusOutlined /> Add</a-button
                    >
                  </div>
                  <a-row :gutter="[16, 16]">
                    <a-col
                      v-for="(comms, tag) in groupedTrackedItems"
                      :key="tag"
                      :xs="24"
                      :md="12"
                      :xl="8"
                    >
                      <div
                        style="
                          padding: 12px;
                          border: 1px solid #f0f0f0;
                          border-radius: 8px;
                          height: 100%;
                        "
                      >
                        <div
                          style="
                            margin-bottom: 8px;
                            border-bottom: 1px solid #f5f5f5;
                            padding-bottom: 4px;
                          "
                        >
                          <a-typography-text strong>{{
                            tag
                          }}</a-typography-text>
                        </div>
                        <div style="display: flex; flex-wrap: wrap; gap: 6px">
                          <a-tag
                            v-for="comm in comms"
                            :key="comm"
                            closable
                            @close.prevent="removeComm(comm)"
                            :color="getCategoryColor(tag as string)"
                            >{{ comm }}</a-tag
                          >
                        </div>
                      </div>
                    </a-col>
                  </a-row>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>

          <!-- Sub-tab 1.3: Tracked Paths -->
          <a-tab-pane key="paths" tab="Paths & Prefixes">
            <a-row :gutter="[24, 24]">
              <a-col :xs="24" :lg="12">
                <a-card title="Exact File Paths" size="small">
                  <template #extra><FolderOutlined /></template>
                  <div
                    style="
                      margin-bottom: 16px;
                      background: #fafafa;
                      padding: 12px;
                      border-radius: 8px;
                      display: flex;
                      gap: 8px;
                    "
                  >
                    <a-input
                      v-model:value="newPathName"
                      placeholder="Absolute path"
                      style="flex: 2"
                    />
                    <a-select
                      v-model:value="newPathTag"
                      style="flex: 1"
                      placeholder="Tag"
                    >
                      <a-select-option
                        v-for="tag in tags"
                        :key="tag"
                        :value="tag"
                        >{{ tag }}</a-select-option
                      >
                    </a-select>
                    <a-button type="primary" @click="addPath"
                      ><PlusOutlined
                    /></a-button>
                  </div>
                  <div
                    v-for="(paths, tag) in groupedTrackedPaths"
                    :key="tag"
                    style="margin-bottom: 12px"
                  >
                    <div style="margin-bottom: 4px">
                      <a-typography-text strong>{{ tag }}</a-typography-text>
                    </div>
                    <div style="display: flex; flex-wrap: wrap; gap: 6px">
                      <a-tag
                        v-for="p in paths"
                        :key="p"
                        closable
                        @close.prevent="removePath(p)"
                        :color="getCategoryColor(tag as string)"
                        >{{ p }}</a-tag
                      >
                    </div>
                  </div>
                </a-card>
              </a-col>

              <a-col :xs="24" :lg="12">
                <a-card title="Path Prefixes (LPM)" size="small">
                  <template #extra><FolderOutlined /></template>
                  <div
                    style="
                      margin-bottom: 16px;
                      background: #fafafa;
                      padding: 12px;
                      border-radius: 8px;
                      display: flex;
                      gap: 8px;
                    "
                  >
                    <a-input
                      v-model:value="newPrefixValue"
                      placeholder="Path prefix (e.g. /etc)"
                      style="flex: 2"
                    />
                    <a-select
                      v-model:value="newPrefixTag"
                      style="flex: 1"
                      placeholder="Tag"
                    >
                      <a-select-option
                        v-for="tag in tags"
                        :key="tag"
                        :value="tag"
                        >{{ tag }}</a-select-option
                      >
                    </a-select>
                    <a-button type="primary" @click="addPrefix"
                      ><PlusOutlined
                    /></a-button>
                  </div>
                  <a-alert
                    type="info"
                    show-icon
                    style="margin-bottom: 12px"
                    message="Prefix matching applies to descendant paths."
                  />
                  <div
                    v-for="(prefixes, tag) in groupedTrackedPrefixes"
                    :key="tag"
                    style="margin-bottom: 12px"
                  >
                    <div style="margin-bottom: 4px">
                      <a-typography-text strong>{{ tag }}</a-typography-text>
                    </div>
                    <div style="display: flex; flex-wrap: wrap; gap: 6px">
                      <a-tag
                        v-for="prefix in prefixes"
                        :key="prefix"
                        closable
                        @close.prevent="removePrefix(prefix)"
                        :color="getCategoryColor(tag as string)"
                        >{{ prefix }}</a-tag
                      >
                    </div>
                  </div>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>
        </a-tabs>
      </a-tab-pane>

      <!-- Tab 2: Security Policies -->
      <a-tab-pane key="security" tab="Security Policies">
        <template #tab>
          <span><SafetyCertificateOutlined /> Security Policies</span>
        </template>
        <a-row :gutter="[24, 24]">
          <a-col :span="24">
            <a-card title="Wrapper Security Policies" size="small">
              <template #extra><SafetyCertificateOutlined /></template>
              <div
                style="
                  margin-bottom: 16px;
                  background: #fafafa;
                  padding: 16px;
                  border-radius: 8px;
                "
              >
                <a-row :gutter="16" align="middle">
                  <a-col :xs="24" :md="6">
                    <a-input
                      v-model:value="newRuleComm"
                      placeholder="Command to intercept (e.g. rm)"
                    />
                  </a-col>
                  <a-col :xs="24" :md="4">
                    <a-select v-model:value="newRuleAction" style="width: 100%">
                      <a-select-option value="BLOCK"
                        >Block Execution</a-select-option
                      >
                      <a-select-option value="REWRITE"
                        >Rewrite Command</a-select-option
                      >
                      <a-select-option value="ALERT"
                        >Alert Only</a-select-option
                      >
                    </a-select>
                  </a-col>
                  <a-col :xs="24" :md="10">
                    <a-input
                      v-if="newRuleAction === 'REWRITE'"
                      v-model:value="newRuleRewritten"
                      placeholder="Rewritten command (e.g. ls -la)"
                    />
                    <span v-else style="color: #999; font-size: 12px"
                      >Intercepts and blocks or warns when the command is called
                      via agent-wrapper</span
                    >
                  </a-col>
                  <a-col :xs="24" :md="4" style="text-align: right">
                    <a-button type="primary" @click="saveRule"
                      ><PlusOutlined /> Add Policy</a-button
                    >
                  </a-col>
                </a-row>
              </div>

              <a-table
                :dataSource="Object.values(wrapperRules)"
                size="small"
                rowKey="comm"
                :pagination="false"
              >
                <a-table-column
                  title="Intercepted Command"
                  data-index="comm"
                  key="comm"
                >
                  <template #default="{ text }"
                    ><code>{{ text }}</code></template
                  >
                </a-table-column>
                <a-table-column title="Action" data-index="action" key="action">
                  <template #default="{ text }">
                    <a-tag
                      :color="
                        text === 'BLOCK'
                          ? 'red'
                          : text === 'REWRITE'
                            ? 'blue'
                            : 'orange'
                      "
                    >
                      <component
                        :is="
                          text === 'BLOCK'
                            ? StopOutlined
                            : text === 'REWRITE'
                              ? SwapOutlined
                              : AlertOutlined
                        "
                      />
                      {{ text }}
                    </a-tag>
                  </template>
                </a-table-column>
                <a-table-column
                  title="Rewritten To"
                  data-index="rewritten_cmd"
                  key="rewritten_cmd"
                >
                  <template #default="{ text }">
                    <code v-if="text && text.length">{{ text.join(" ") }}</code>
                    <span v-else>-</span>
                  </template>
                </a-table-column>
                <a-table-column title="Remove" key="action" width="100px">
                  <template #default="{ record }">
                    <a-button
                      type="link"
                      danger
                      @click="deleteRule(record.comm)"
                      >Delete</a-button
                    >
                  </template>
                </a-table-column>
              </a-table>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- Tab 3: System & Runtime -->
      <a-tab-pane key="system" tab="System & Runtime">
        <template #tab>
          <span><ReloadOutlined /> System & Runtime</span>
        </template>
        <a-row :gutter="[24, 24]">
          <a-col :span="24">
            <a-card title="Runtime & MCP Access" size="small">
              <template #extra>
                <SafetyCertificateOutlined />
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div style="display: flex; align-items: center; gap: 12px">
                      <a-switch
                        v-model:checked="runtimeSettings.logPersistenceEnabled"
                      />
                      <span>Persist captured logs to file</span>
                    </div>
                    <a-input
                      v-model:value="runtimeSettings.logFilePath"
                      placeholder="Log file path (defaults to ~/.config/agent-ebpf-filter/events.jsonl)"
                    />
                    <div
                      style="
                        display: flex;
                        gap: 8px;
                        flex-wrap: wrap;
                        align-items: center;
                      "
                    >
                      <a-button type="primary" @click="saveRuntime">
                        <ReloadOutlined /> Save Runtime
                      </a-button>
                      <a-tag :color="persistedEventLogAlive ? 'green' : 'red'">
                        {{
                          persistedEventLogAlive
                            ? "Log file ready"
                            : "Log file inactive"
                        }}
                      </a-tag>
                      <a-tag color="blue">{{
                        persistedEventLogPath || "No log path"
                      }}</a-tag>
                    </div>
                    <a-typography-text type="secondary">
                      When enabled, new events are appended as JSONL and can be
                      exported or tailed through MCP.
                    </a-typography-text>
                  </div>
                </a-col>
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div>
                      <div style="margin-bottom: 6px; font-weight: 600">
                        Access Token
                      </div>
                      <a-input
                        :value="runtimeSettings.accessToken"
                        readonly
                        placeholder="Generate a token to access /config and /mcp"
                      />
                      <div
                        style="
                          display: flex;
                          gap: 8px;
                          flex-wrap: wrap;
                          margin-top: 8px;
                        "
                      >
                        <a-button @click="rotateAccessToken">
                          <ReloadOutlined /> Generate / Rotate
                        </a-button>
                        <a-button
                          @click="
                            copyText(
                              runtimeSettings.accessToken,
                              'Access token copied',
                            )
                          "
                        >
                          <CopyOutlined /> Copy Token
                        </a-button>
                      </div>
                    </div>
                    <div
                      style="display: flex; flex-direction: column; gap: 8px"
                    >
                      <div style="margin-bottom: 2px; font-weight: 600">
                        MCP Endpoint
                      </div>
                      <a-input :value="mcpEndpoint" readonly />
                      <div style="display: flex; gap: 8px; flex-wrap: wrap">
                        <a-button
                          @click="copyText(mcpEndpoint, 'MCP endpoint copied')"
                        >
                          <CopyOutlined /> Copy Base URL
                        </a-button>
                      </div>
                      <div style="margin-top: 4px; font-weight: 600">
                        MCP Query URL
                      </div>
                      <a-input :value="mcpQueryEndpoint" readonly />
                      <div style="display: flex; gap: 8px; flex-wrap: wrap">
                        <a-button
                          @click="
                            copyText(mcpQueryEndpoint, 'MCP query URL copied')
                          "
                        >
                          <CopyOutlined /> Copy Query URL
                        </a-button>
                        <a-button
                          @click="
                            copyText(
                              mcpQueryEndpointTemplate,
                              'MCP query template copied',
                            )
                          "
                        >
                          <CopyOutlined /> Copy Template
                        </a-button>
                      </div>
                      <a-alert
                        type="success"
                        show-icon
                        :message="'Query URL is generated live from the current token and updates when you rotate it.'"
                        style="margin-top: 4px"
                      />
                    </div>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col :span="24">
            <a-card title="Data Management" size="small">
              <template #extra>
                <DeleteOutlined />
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div style="font-weight: 600">Event Retention</div>
                    <div style="display: flex; align-items: center; gap: 12px">
                      <span>Max in-memory events:</span>
                      <a-input-number
                        v-model:value="runtimeSettings.maxEventCount"
                        :min="100"
                        :max="10000"
                        :step="100"
                        style="width: 140px"
                      />
                    </div>
                    <div
                      style="
                        display: flex;
                        align-items: center;
                        gap: 12px;
                        flex-wrap: wrap;
                      "
                    >
                      <span>Max event age:</span>
                      <a-input
                        v-model:value="runtimeSettings.maxEventAge"
                        placeholder="e.g. 24h, 168h, 0 = no limit"
                        style="width: 200px"
                      />
                      <a-typography-text type="secondary">
                        Go duration format (24h, 30m, 168h)
                      </a-typography-text>
                    </div>
                    <div
                      style="
                        display: flex;
                        gap: 8px;
                        flex-wrap: wrap;
                        align-items: center;
                      "
                    >
                      <a-button type="primary" @click="saveRuntime">
                        <ReloadOutlined /> Save Retention
                      </a-button>
                    </div>
                  </div>
                </a-col>
                <a-col :xs="24" :md="12">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div style="font-weight: 600">Manual Cleanup</div>
                    <div style="display: flex; gap: 8px; flex-wrap: wrap">
                      <a-popconfirm
                        title="Clear in-memory event buffer?"
                        @confirm="clearInMemoryEvents"
                      >
                        <a-button size="small" danger
                          >Clear Memory Events</a-button
                        >
                      </a-popconfirm>
                      <a-popconfirm
                        title="Truncate persisted event log file?"
                        @confirm="clearPersistedLog"
                      >
                        <a-button size="small" danger
                          >Truncate Log File</a-button
                        >
                      </a-popconfirm>
                      <a-popconfirm
                        title="Clear all events (memory + file)?"
                        @confirm="clearAllEvents"
                      >
                        <a-button size="small" type="primary" danger
                          >Clear All Events</a-button
                        >
                      </a-popconfirm>
                    </div>
                    <a-typography-text type="secondary">
                      These actions are irreversible. Memory events and/or the
                      JSONL log file will be permanently deleted.
                    </a-typography-text>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- Tab 4: Cluster Control -->
      <a-tab-pane key="cluster" tab="Cluster Control">
        <template #tab>
          <span><ClusterOutlined /> Cluster Control</span>
        </template>
        <a-row :gutter="[24, 24]">
          <a-col :span="24">
            <a-card title="Cluster Status" size="small">
              <template #extra>
                <ClusterOutlined />
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="10">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div
                      style="
                        display: flex;
                        align-items: center;
                        gap: 8px;
                        flex-wrap: wrap;
                      "
                    >
                      <span style="font-weight: 600">Mode</span>
                      <a-tag :color="clusterRoleColor">{{
                        clusterRoleText
                      }}</a-tag>
                      <a-tag
                        :color="
                          clusterState?.role === 'slave' ? 'orange' : 'green'
                        "
                      >
                        {{
                          clusterState?.role === "slave"
                            ? "Managed by master_url"
                            : "Default master mode"
                        }}
                      </a-tag>
                    </div>
                    <a-descriptions bordered size="small" :column="1">
                      <a-descriptions-item label="Node ID">
                        <span class="cluster-value">{{
                          clusterState?.nodeId || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item label="Node Name">
                        <span class="cluster-value">{{
                          clusterState?.nodeName || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item label="Node URL">
                        <span class="cluster-value">{{
                          clusterState?.nodeUrl || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item
                        v-if="clusterState?.role === 'slave'"
                        label="Master URL"
                      >
                        <span class="cluster-value">{{
                          clusterState?.masterUrl || "—"
                        }}</span>
                      </a-descriptions-item>
                      <a-descriptions-item label="Cluster Auth">
                        <span>
                          {{
                            clusterState?.accountConfigured
                              ? "account set"
                              : "account missing"
                          }}
                          /
                          {{
                            clusterState?.passwordConfigured
                              ? "password set"
                              : "password missing"
                          }}
                        </span>
                      </a-descriptions-item>
                    </a-descriptions>
                  </div>
                </a-col>
                <a-col :xs="24" :md="14">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <a-alert
                      type="info"
                      show-icon
                      message="Select the backend you want to inspect. All API/WS traffic is forwarded by the master."
                    />
                    <div
                      style="
                        display: flex;
                        gap: 12px;
                        align-items: center;
                        flex-wrap: wrap;
                      "
                    >
                      <span style="font-weight: 600">Active Target</span>
                      <a-select
                        v-model:value="selectedClusterTarget"
                        :options="clusterNodeOptions"
                        style="min-width: 280px; flex: 1"
                        :disabled="clusterState?.role === 'slave'"
                        @change="applyClusterTarget"
                      />
                      <a-button @click="fetchClusterNodes">
                        <ReloadOutlined /> Refresh Nodes
                      </a-button>
                    </div>
                    <a-table
                      :data-source="clusterNodes"
                      row-key="id"
                      size="small"
                      :pagination="false"
                      :row-class-name="getClusterRowClass"
                    >
                      <a-table-column title="Name" data-index="name" key="name">
                        <template #default="{ text, record }">
                          <span style="font-weight: 600">{{ text }}</span>
                          <a-tag
                            v-if="record.isLocal"
                            color="green"
                            style="margin-left: 8px"
                            >local</a-tag
                          >
                        </template>
                      </a-table-column>
                      <a-table-column title="Role" data-index="role" key="role">
                        <template #default="{ text }">
                          <a-tag
                            :color="text === 'slave' ? 'orange' : 'green'"
                            >{{ text }}</a-tag
                          >
                        </template>
                      </a-table-column>
                      <a-table-column
                        title="Status"
                        data-index="status"
                        key="status"
                      >
                        <template #default="{ text }">
                          <a-tag
                            :color="
                              text === 'online'
                                ? 'green'
                                : text === 'stale'
                                  ? 'orange'
                                  : 'default'
                            "
                            >{{ text }}</a-tag
                          >
                        </template>
                      </a-table-column>
                      <a-table-column title="URL" data-index="url" key="url">
                        <template #default="{ text }">
                          <span class="cluster-url">{{ text }}</span>
                        </template>
                      </a-table-column>
                      <a-table-column
                        title="Last Seen"
                        data-index="lastSeen"
                        key="lastSeen"
                      >
                        <template #default="{ text }">
                          <span>{{
                            text ? new Date(text).toLocaleString() : "—"
                          }}</span>
                        </template>
                      </a-table-column>
                      <a-table-column title="Action" key="action" width="120px">
                        <template #default="{ record }">
                          <a-button
                            v-if="
                              !record.isLocal && clusterState?.role === 'master'
                            "
                            type="link"
                            @click="applyClusterTarget(record.id)"
                          >
                            Route here
                          </a-button>
                          <span v-else style="color: #999">—</span>
                        </template>
                      </a-table-column>
                    </a-table>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<style scoped>
:deep(.ant-card) {
  border-radius: 8px;
}
.cluster-value {
  display: block;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid #dbeafe;
  background: linear-gradient(180deg, #f8fbff 0%, #eef4ff 100%);
  color: #1f2937;
  font-family: var(--mono);
  word-break: break-all;
}
.cluster-url {
  display: inline-block;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  background: #f8fafc;
  color: #111827;
  font-family: var(--mono);
  word-break: break-all;
  white-space: normal;
}
:deep(.cluster-row-active > td) {
  background: #f0f9eb !important;
}
:deep(.cluster-row-local > td) {
  background: #fafcff !important;
}
</style>
