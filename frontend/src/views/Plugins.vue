<script setup lang="ts">
import { ref, onMounted, computed, watch } from "vue";
import {
  AppstoreOutlined,
  CodeOutlined,
  PlusOutlined,
  ReloadOutlined,
  DeleteOutlined,
  ThunderboltOutlined,
  SaveOutlined,
  PoweroffOutlined,
} from "@ant-design/icons-vue";
import { Modal, message } from "ant-design-vue";
import { useRoute, useRouter } from "vue-router";
import {
  usePlugins,
  type PluginManifest,
  type PluginAttachKind,
  type BPFTemplate,
} from "../composables/usePlugins";
import PluginsPseudoCodeTab from "../components/plugins/PluginsPseudoCodeTab.vue";
import PluginsVisualTab from "../components/plugins/PluginsVisualTab.vue";

const {
  plugins,
  templates,
  compileLog,
  lastCompile,
  loading,
  fetchPlugins,
  fetchTemplates,
  fetchPlugin,
  upsertPlugin,
  deletePlugin,
  togglePlugin,
  compileBpf,
  loadBpf,
  unloadBpf,
} = usePlugins();

const route = useRoute();
const router = useRouter();

const pluginTabKeys = new Set(["list", "builder", "visual", "pseudo"]);
const normalizePluginTab = (
  tab: unknown
): "list" | "builder" | "visual" | "pseudo" =>
  typeof tab === "string" && pluginTabKeys.has(tab)
    ? (tab as "list" | "builder" | "visual" | "pseudo")
    : "list";

const activeTab = ref<"list" | "builder" | "visual" | "pseudo">(
  normalizePluginTab(route.params.tab)
);

// ─── Builder state ────────────────────────────────────────────────
const builder = ref({
  id: "",
  name: "",
  description: "",
  author: "",
  version: "1.0.0",
  attachKind: "tracepoint" as PluginAttachKind,
  attachTarget: "syscalls/sys_enter_execve",
  programName: "trace_execve",
  enabled: false,
  source: "",
});

const compiling = ref(false);
const saving = ref(false);

const attachKindOptions = [
  { value: "tracepoint", label: "Tracepoint" },
  { value: "kprobe", label: "Kprobe" },
  { value: "kretprobe", label: "Kretprobe" },
  { value: "lsm", label: "BPF LSM" },
];

const applyTemplate = (tpl: BPFTemplate) => {
  builder.value.id = builder.value.id || tpl.id;
  builder.value.name = builder.value.name || tpl.name;
  builder.value.description = builder.value.description || tpl.description;
  builder.value.attachKind = tpl.attachKind;
  builder.value.attachTarget = tpl.attachTarget;
  builder.value.programName = tpl.programName;
  builder.value.source = tpl.source;
  message.info(`已加载模板：${tpl.name}`);
};

const resetBuilder = () => {
  builder.value = {
    id: "",
    name: "",
    description: "",
    author: "",
    version: "1.0.0",
    attachKind: "tracepoint",
    attachTarget: "syscalls/sys_enter_execve",
    programName: "trace_execve",
    enabled: false,
    source: "",
  };
  compileLog.value = "";
  lastCompile.value = null;
};

const handleCompile = async () => {
  if (!builder.value.id) {
    message.warning("请填写插件 ID");
    return;
  }
  compiling.value = true;
  await compileBpf(builder.value.id, builder.value.source);
  compiling.value = false;
};

const handleSave = async () => {
  if (!builder.value.id || !builder.value.name) {
    message.warning("请填写 ID 与名称");
    return;
  }
  saving.value = true;
  const payload = {
    id: builder.value.id,
    name: builder.value.name,
    description: builder.value.description,
    author: builder.value.author,
    version: builder.value.version,
    kind: "ebpf" as const,
    enabled: builder.value.enabled,
    attachKind: builder.value.attachKind,
    attachTarget: builder.value.attachTarget,
    programName: builder.value.programName,
    source: builder.value.source,
  };
  const saved = await upsertPlugin(payload);
  saving.value = false;
  if (saved) {
    activeTab.value = "list";
  }
};

const handleLoadIntoBuilder = async (id: string) => {
  const data = await fetchPlugin(id);
  if (!data) return;
  const { plugin, source } = data;
  builder.value = {
    id: plugin.id,
    name: plugin.name,
    description: plugin.description || "",
    author: plugin.author || "",
    version: plugin.version || "1.0.0",
    attachKind: (plugin.attachKind || "tracepoint") as PluginAttachKind,
    attachTarget: plugin.attachTarget || "",
    programName: plugin.programName || "",
    enabled: !!plugin.enabled,
    source,
  };
  activeTab.value = "builder";
};

const confirmDelete = (plugin: PluginManifest) => {
  Modal.confirm({
    title: `删除插件 ${plugin.id}?`,
    content: "该操作会卸载已加载的 eBPF 程序并删除全部源码与编译产物。",
    okType: "danger",
    onOk: () => deletePlugin(plugin.id),
  });
};

const handleToggle = async (plugin: PluginManifest, value: boolean) => {
  await togglePlugin(plugin.id, value);
};

const kindLabel = (kind: string) => {
  switch (kind) {
    case "ebpf":
      return "eBPF";
    case "webhook":
      return "Webhook";
    case "command":
      return "命令规则";
    default:
      return kind;
  }
};

const statusTag = (plugin: PluginManifest) => {
  if (!plugin.enabled) return { color: "default", text: "已禁用" };
  if (plugin.loadError) return { color: "red", text: "加载失败" };
  if (plugin.loaded) return { color: "green", text: "运行中" };
  return { color: "orange", text: "已启用" };
};

const sortedPlugins = computed(() =>
  [...plugins.value].sort((a, b) => a.id.localeCompare(b.id))
);

watch(
  () => route.params.tab,
  (tab) => {
    activeTab.value = normalizePluginTab(tab);
  },
  { immediate: true }
);

watch(activeTab, (tab) => {
  if (tab !== normalizePluginTab(route.params.tab)) {
    router.replace({ name: "Plugins", params: { tab } });
  }
});

onMounted(async () => {
  await fetchPlugins();
  await fetchTemplates();
});

watch(
  () => builder.value.programName,
  (val) => {
    if (val && !builder.value.id) {
      builder.value.id = val.replace(/_/g, "-").toLowerCase();
    }
  }
);
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%">
    <a-tabs v-model:activeKey="activeTab" type="card" size="large">
      <!-- ─── 插件列表 ─── -->
      <a-tab-pane key="list">
        <template #tab
          ><span><AppstoreOutlined /> 插件管理</span></template
        >

        <a-card>
          <template #title>
            <span>已注册的插件 ({{ plugins.length }})</span>
          </template>
          <template #extra>
            <a-space>
              <a-button @click="fetchPlugins" :loading="loading">
                <template #icon><ReloadOutlined /></template>
                刷新
              </a-button>
              <a-button
                type="primary"
                @click="
                  activeTab = 'builder';
                  resetBuilder();
                "
              >
                <template #icon><PlusOutlined /></template>
                新建插件
              </a-button>
            </a-space>
          </template>

          <a-empty
            v-if="!sortedPlugins.length"
            description="暂无插件，前往“在线 eBPF 制作”创建一个"
          />

          <a-list v-else :data-source="sortedPlugins" item-layout="vertical">
            <template #renderItem="{ item }">
              <a-list-item :key="item.id">
                <template #actions>
                  <a-switch
                    :checked="item.enabled"
                    @change="(v: boolean) => handleToggle(item, v)"
                    checked-children="启用"
                    un-checked-children="停用"
                  />
                  <a-button
                    size="small"
                    @click="handleLoadIntoBuilder(item.id)"
                  >
                    <template #icon><CodeOutlined /></template>
                    编辑
                  </a-button>
                  <a-button size="small" danger @click="confirmDelete(item)">
                    <template #icon><DeleteOutlined /></template>
                    删除
                  </a-button>
                </template>
                <a-list-item-meta>
                  <template #title>
                    <a-space>
                      <span style="font-size: 16px; font-weight: 600">{{
                        item.name
                      }}</span>
                      <a-tag color="blue">{{ kindLabel(item.kind) }}</a-tag>
                      <a-tag :color="statusTag(item).color">{{
                        statusTag(item).text
                      }}</a-tag>
                      <a-tag v-if="item.attachKind"
                        >{{ item.attachKind }}: {{ item.attachTarget }}</a-tag
                      >
                    </a-space>
                  </template>
                  <template #description>
                    <div>{{ item.description || "（无说明）" }}</div>
                    <div style="color: #999; font-size: 12px; margin-top: 4px">
                      ID: <code>{{ item.id }}</code>
                      <span v-if="item.programName">
                        · 程序: <code>{{ item.programName }}</code></span
                      >
                      <span v-if="item.version"> · v{{ item.version }}</span>
                      <span v-if="item.author"> · {{ item.author }}</span>
                    </div>
                    <a-alert
                      v-if="item.loadError"
                      style="margin-top: 8px"
                      type="error"
                      :message="item.loadError"
                      show-icon
                    />
                  </template>
                </a-list-item-meta>
              </a-list-item>
            </template>
          </a-list>
        </a-card>
      </a-tab-pane>

      <!-- ─── 在线 eBPF 制作 ─── -->
      <a-tab-pane key="builder">
        <template #tab
          ><span><CodeOutlined /> 在线 eBPF 制作</span></template
        >

        <a-row :gutter="16">
          <a-col :span="7">
            <a-card title="模板库" size="small">
              <template #extra>
                <a-button size="small" @click="fetchTemplates">
                  <template #icon><ReloadOutlined /></template>
                </a-button>
              </template>
              <a-list :data-source="templates" size="small">
                <template #renderItem="{ item }">
                  <a-list-item :key="item.id">
                    <a-list-item-meta>
                      <template #title>
                        <a
                          @click="applyTemplate(item)"
                          style="font-weight: 600"
                          >{{ item.name }}</a
                        >
                      </template>
                      <template #description>
                        <div style="font-size: 12px">
                          {{ item.description }}
                        </div>
                        <a-tag style="margin-top: 4px"
                          >{{ item.attachKind }}: {{ item.attachTarget }}</a-tag
                        >
                      </template>
                    </a-list-item-meta>
                  </a-list-item>
                </template>
              </a-list>
            </a-card>

            <a-card title="元数据" size="small" style="margin-top: 12px">
              <a-form layout="vertical" :model="builder">
                <a-form-item label="插件 ID (a-z0-9-)">
                  <a-input
                    v-model:value="builder.id"
                    placeholder="例如 my-trace-exec"
                  />
                </a-form-item>
                <a-form-item label="名称">
                  <a-input v-model:value="builder.name" />
                </a-form-item>
                <a-form-item label="说明">
                  <a-textarea v-model:value="builder.description" :rows="2" />
                </a-form-item>
                <a-row :gutter="8">
                  <a-col :span="12">
                    <a-form-item label="作者">
                      <a-input v-model:value="builder.author" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="版本">
                      <a-input v-model:value="builder.version" />
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-form-item label="附挂类型">
                  <a-select
                    v-model:value="builder.attachKind"
                    :options="attachKindOptions"
                  />
                </a-form-item>
                <a-form-item label="附挂目标">
                  <a-input
                    v-model:value="builder.attachTarget"
                    :placeholder="
                      builder.attachKind === 'tracepoint'
                        ? 'syscalls/sys_enter_xxx'
                        : 'do_unlinkat'
                    "
                  />
                </a-form-item>
                <a-form-item label="程序入口 (SEC 名)">
                  <a-input
                    v-model:value="builder.programName"
                    placeholder="例如 trace_execve"
                  />
                </a-form-item>
                <a-form-item>
                  <a-checkbox v-model:checked="builder.enabled"
                    >保存后立即启用</a-checkbox
                  >
                </a-form-item>
              </a-form>
            </a-card>
          </a-col>

          <a-col :span="17">
            <a-card title="源码 (C / libbpf)" size="small">
              <template #extra>
                <a-space>
                  <a-button :loading="compiling" @click="handleCompile">
                    <template #icon><ThunderboltOutlined /></template>
                    编译
                  </a-button>
                  <a-button
                    :loading="saving"
                    type="primary"
                    @click="handleSave"
                  >
                    <template #icon><SaveOutlined /></template>
                    保存
                  </a-button>
                  <a-button v-if="lastCompile" @click="loadBpf(builder.id)">
                    <template #icon><PoweroffOutlined /></template>
                    立即加载
                  </a-button>
                  <a-button
                    v-if="lastCompile"
                    danger
                    @click="unloadBpf(builder.id)"
                    >卸载</a-button
                  >
                </a-space>
              </template>
              <a-textarea
                v-model:value="builder.source"
                :rows="20"
                placeholder="选择左侧模板，或在此输入 libbpf C 源码"
                style="
                  font-family: 'JetBrains Mono', Menlo, Consolas, monospace;
                  font-size: 12px;
                "
              />
              <a-alert
                v-if="lastCompile"
                style="margin-top: 12px"
                type="success"
                :message="`编译成功 · ${lastCompile.objectPath}`"
                :description="`SHA-256: ${lastCompile.sourceSha256}`"
                show-icon
              />
              <a-card
                v-if="compileLog"
                size="small"
                title="编译输出"
                style="margin-top: 12px; background: #1f1f1f"
              >
                <pre
                  style="
                    margin: 0;
                    color: #fafafa;
                    font-size: 12px;
                    max-height: 240px;
                    overflow: auto;
                  "
                  >{{ compileLog }}</pre
                >
              </a-card>
            </a-card>

            <a-card size="small" style="margin-top: 12px" title="编写提示">
              <ul style="margin: 0; padding-left: 20px; color: #666">
                <li>
                  仅允许 <code>#include &quot;vmlinux.h&quot;</code> 与
                  <code>&lt;bpf/...&gt;</code> 形式的头文件。
                </li>
                <li>
                  必须声明至少一个 <code>SEC(&quot;...&quot;)</code> 程序。
                </li>
                <li>
                  需要在主机上安装 <code>clang &ge; 14</code>，并允许后端访问
                  <code>/sys/fs/bpf</code>。
                </li>
                <li>
                  Tracepoint 目标格式：<code>category/name</code>，例如
                  <code>syscalls/sys_enter_openat</code>。
                </li>
                <li>
                  Kprobe / Kretprobe 目标为函数名，例如
                  <code>do_unlinkat</code>。
                </li>
              </ul>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- ─── 可视化积木制作 ─── -->
      <a-tab-pane key="visual">
        <template #tab
          ><span><ThunderboltOutlined /> 可视化积木制作</span></template
        >
        <PluginsVisualTab />
      </a-tab-pane>

      <!-- ─── 独立 TS 伪代码制作 ─── -->
      <a-tab-pane key="pseudo">
        <template #tab><span><CodeOutlined /> TS 伪代码制作</span></template>
        <PluginsPseudoCodeTab />
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<style scoped>
:deep(.ant-card) {
  border-radius: 8px;
}
</style>
