<script setup lang="ts">
import { defineAsyncComponent, onMounted, ref, watch } from "vue";
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
import { message } from "ant-design-vue";
import axios from "axios";
import { useRoute, useRouter } from "vue-router";
import { usePlugins } from "../../composables/plugins/usePlugins";
import { usePluginBuilder } from "./usePluginBuilder";
import { usePluginList } from "./usePluginList";

const PluginsPseudoCodeTab = defineAsyncComponent(
  () => import("../../components/plugins/PluginsPseudoCodeTab.vue"),
);
const PluginsVisualTab = defineAsyncComponent(
  () => import("../../components/plugins/PluginsVisualTab.vue"),
);

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

const pluginTabKeys = new Set(["list", "builder", "visual", "pseudo", "javascript"]);
const normalizePluginTab = (
  tab: unknown,
): "list" | "builder" | "visual" | "pseudo" | "javascript" =>
  typeof tab === "string" && pluginTabKeys.has(tab)
    ? (tab as "list" | "builder" | "visual" | "pseudo" | "javascript")
    : "list";

const activeTab = ref<"list" | "builder" | "visual" | "pseudo" | "javascript">(
  normalizePluginTab(route.params.tab),
);

// Plugin builder state and operations
const {
  builder,
  compiling,
  saving,
  attachKindOptions,
  applyTemplate,
  resetBuilder,
  handleCompile,
  handleSave,
  handleLoadIntoBuilder,
} = usePluginBuilder(
  compileBpf,
  upsertPlugin,
  fetchPlugin,
  compileLog,
  lastCompile,
  activeTab,
);


const jsDraft = ref({
  id: "",
  name: "",
  description: "",
  source: "export function onEvent(event) { return { risk: event.risk ?? 0 }; }",
  enabled: false,
});
const jsSaving = ref(false);
const jsRunning = ref(false);
const jsHook = ref<"onEvent" | "onAlert">("onEvent");
const jsInput = ref('{"risk":85,"comm":"agent"}');
const jsOutput = ref("");

function newJSPlugin() {
  jsDraft.value = {
    id: "", name: "", description: "",
    source: "export function onEvent(event) { return { risk: event.risk ?? 0 }; }",
    enabled: false,
  };
  jsOutput.value = "";
  activeTab.value = "javascript";
}
async function editJSPlugin(id: string) {
  const data = await fetchPlugin(id);
  if (!data || data.plugin.kind !== "javascript") return;
  jsDraft.value = {
    id: data.plugin.id,
    name: data.plugin.name,
    description: data.plugin.description || "",
    source: data.source,
    enabled: !!data.plugin.enabled,
  };
  jsOutput.value = "";
  activeTab.value = "javascript";
}
async function saveJSPlugin() {
  if (!jsDraft.value.id || !jsDraft.value.name || !jsDraft.value.source) {
    message.error("请填写插件 ID、名称和 JavaScript 源码");
    return;
  }
  jsSaving.value = true;
  try {
    const enableAfterSave = jsDraft.value.enabled;
    const plugin = await upsertPlugin({
      id: jsDraft.value.id,
      name: jsDraft.value.name,
      description: jsDraft.value.description,
      kind: "javascript",
      enabled: false,
      source: jsDraft.value.source,
    });
    if (!plugin) return;
    jsDraft.value.enabled = false;
    if (enableAfterSave) {
      const enabled = await togglePlugin(plugin.id, true);
      jsDraft.value.enabled = !!enabled?.enabled;
    }
  } finally {
    jsSaving.value = false;
  }
}
async function invokeJSPlugin() {
  if (!jsDraft.value.enabled) {
    message.warning("请先保存并启用 JavaScript 插件");
    return;
  }
  let payload: unknown;
  try {
    payload = JSON.parse(jsInput.value);
  } catch {
    message.error("请输入有效的 JSON");
    return;
  }
  jsRunning.value = true;
  jsOutput.value = "";
  try {
    const url = `/plugins/${encodeURIComponent(jsDraft.value.id)}/invoke`;
    const response = await axios.post(url, { hook: jsHook.value, payload });
    jsOutput.value = JSON.stringify(response.data.result, null, 2);
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || "插件执行失败");
  } finally {
    jsRunning.value = false;
  }
}

// Plugin list helpers
const { kindLabel, statusTag, sortedPlugins, confirmDelete, handleToggle } =
  usePluginList(plugins, deletePlugin, togglePlugin);

watch(
  () => route.params.tab,
  (tab) => {
    activeTab.value = normalizePluginTab(tab);
  },
  { immediate: true },
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
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%">
    <a-tabs v-model:activeKey="activeTab" type="card" size="large">
      <!-- --- Plugin List --- -->
      <a-tab-pane key="list">
        <template #tab
          ><span><AppstoreOutlined /> Plugin Management</span></template
        >

        <a-card>
          <template #title>
            <span>Registered Plugins ({{ plugins.length }})</span>
          </template>
          <template #extra>
            <a-space>
              <a-button @click="fetchPlugins" :loading="loading">
                <template #icon><ReloadOutlined /></template>
                Refresh
              </a-button>
              <a-button @click="newJSPlugin">New JavaScript Plugin</a-button>
              <a-button
                type="primary"
                @click="
                  activeTab = 'builder';
                  resetBuilder();
                "
              >
                <template #icon><PlusOutlined /></template>
                New Plugin
              </a-button>
            </a-space>
          </template>

          <a-empty
            v-if="!sortedPlugins.length"
            description="No plugins registered yet."
          />

          <a-list v-else :data-source="sortedPlugins" item-layout="vertical">
            <template #renderItem="{ item }">
              <a-list-item :key="item.id">
                <template #actions>
                  <a-switch
                    :checked="item.enabled"
                    @change="(v: boolean) => handleToggle(item, v)"
                    checked-children="ON"
                    un-checked-children="OFF"
                  />
                  <a-button
                    size="small"
                    @click="item.kind === 'javascript' ? editJSPlugin(item.id) : handleLoadIntoBuilder(item.id)"
                  >
                    <template #icon><CodeOutlined /></template>
                    Edit
                  </a-button>
                  <a-button size="small" danger @click="confirmDelete(item)">
                    <template #icon><DeleteOutlined /></template>
                    Delete
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
                    <div>{{ item.description || "(no description)" }}</div>
                    <div style="color: #6b7280; font-size: 12px; margin-top: 4px">
                      ID: <code>{{ item.id }}</code>
                      <span v-if="item.programName">
                        · Program: <code>{{ item.programName }}</code></span
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


      <!-- --- JavaScript metadata plugin editor --- -->
      <a-tab-pane key="javascript">
        <template #tab><span><CodeOutlined /> JavaScript Plugin</span></template>
        <a-card title="moejs / JavaScript Metadata Hook" size="small">
          <a-alert
            type="warning"
            show-icon
            message="实验性：仅手动调用，不自动订阅 eBPF；不能作为内核安全边界，请仅使用可信插件。"
            style="margin-bottom: 16px"
          />
          <a-row :gutter="16">
            <a-col :xs="24" :lg="8">
              <a-form layout="vertical">
                <a-form-item label="Plugin ID (a-z0-9-)">
                  <a-input v-model:value="jsDraft.id" placeholder="risk-annotation" />
                </a-form-item>
                <a-form-item label="Name">
                  <a-input v-model:value="jsDraft.name" />
                </a-form-item>
                <a-form-item label="Description">
                  <a-textarea v-model:value="jsDraft.description" :rows="2" />
                </a-form-item>
                <a-form-item>
                  <a-checkbox v-model:checked="jsDraft.enabled">Enable after save</a-checkbox>
                </a-form-item>
              </a-form>
              <a-button type="primary" :loading="jsSaving" @click="saveJSPlugin">
                <template #icon><SaveOutlined /></template> Save JavaScript Plugin
              </a-button>
            </a-col>
            <a-col :xs="24" :lg="16">
              <a-form-item label="ES module source: export onEvent / onAlert">
                <a-textarea v-model:value="jsDraft.source" :rows="13" style="font-family: monospace" />
              </a-form-item>
              <a-divider />
              <a-space style="margin-bottom: 10px">
                <a-select
                  v-model:value="jsHook"
                  style="width: 140px"
                  :options="[{ label: 'onEvent', value: 'onEvent' }, { label: 'onAlert', value: 'onAlert' }]"
                />
                <a-button :loading="jsRunning" @click="invokeJSPlugin">Test Hook</a-button>
              </a-space>
              <a-textarea v-model:value="jsInput" :rows="4" style="font-family: monospace" />
              <pre v-if="jsOutput" style="white-space: pre-wrap; margin-top: 10px">{{ jsOutput }}</pre>
            </a-col>
          </a-row>
        </a-card>
      </a-tab-pane>

      <!-- --- Online eBPF Builder --- -->
      <a-tab-pane key="builder">
        <template #tab
          ><span><CodeOutlined /> Online eBPF Builder</span></template
        >

        <a-row :gutter="16">
          <a-col :span="7">
            <a-card title="Templates" size="small">
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

            <a-card title="Metadata" size="small" style="margin-top: 12px">
              <a-form layout="vertical" :model="builder">
                <a-form-item label="Plugin ID (a-z0-9-)">
                  <a-input
                    v-model:value="builder.id"
                    placeholder="e.g. my-trace-exec"
                  />
                </a-form-item>
                <a-form-item label="Name">
                  <a-input v-model:value="builder.name" />
                </a-form-item>
                <a-form-item label="Description">
                  <a-textarea v-model:value="builder.description" :rows="2" />
                </a-form-item>
                <a-row :gutter="8">
                  <a-col :span="12">
                    <a-form-item label="Author">
                      <a-input v-model:value="builder.author" />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="Version">
                      <a-input v-model:value="builder.version" />
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-form-item label="Attach Kind">
                  <a-select
                    v-model:value="builder.attachKind"
                    :options="attachKindOptions"
                  />
                </a-form-item>
                <a-form-item label="Attach Target">
                  <a-input
                    v-model:value="builder.attachTarget"
                    :placeholder="
                      builder.attachKind === 'tracepoint'
                        ? 'syscalls/sys_enter_xxx'
                        : 'do_unlinkat'
                    "
                  />
                </a-form-item>
                <a-form-item label="Program Entry (SEC name)">
                  <a-input
                    v-model:value="builder.programName"
                    placeholder="e.g. trace_execve"
                  />
                </a-form-item>
                <a-form-item>
                  <a-checkbox v-model:checked="builder.enabled"
                    >Enable after save</a-checkbox
                  >
                </a-form-item>
              </a-form>
            </a-card>
          </a-col>

          <a-col :span="17">
            <a-card title="Source (C / libbpf)" size="small">
              <template #extra>
                <a-space>
                  <a-button :loading="compiling" @click="handleCompile">
                    <template #icon><ThunderboltOutlined /></template>
                    Compile
                  </a-button>
                  <a-button
                    :loading="saving"
                    type="primary"
                    @click="handleSave"
                  >
                    <template #icon><SaveOutlined /></template>
                    Save
                  </a-button>
                  <a-button v-if="lastCompile" @click="loadBpf(builder.id)">
                    <template #icon><PoweroffOutlined /></template>
                    Load
                  </a-button>
                  <a-button
                    v-if="lastCompile"
                    danger
                    @click="unloadBpf(builder.id)"
                    >Unload</a-button
                  >
                </a-space>
              </template>
              <a-textarea
                v-model:value="builder.source"
                :rows="20"
                placeholder="Select a template or enter libbpf C source code"
                style="
                  font-family:
                    &quot;JetBrains Mono&quot;, Menlo, Consolas, monospace;
                  font-size: 12px;
                "
              />
              <a-alert
                v-if="lastCompile"
                style="margin-top: 12px"
                type="success"
                :message="`Compiled · ${lastCompile.objectPath}`"
                :description="`SHA-256: ${lastCompile.sourceSha256}`"
                show-icon
              />
              <a-card
                v-if="compileLog"
                size="small"
                title="Compile Output"
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

            <a-card size="small" style="margin-top: 12px" title="Tips">
              <ul style="margin: 0; padding-left: 20px; color: #4a4a4a">
                <li>
                  Only <code>#include &quot;vmlinux.h&quot;</code> and
                  <code>&lt;bpf/...&gt;</code> headers are allowed.
                </li>
                <li>
                  At least one <code>SEC(&quot;...&quot;)</code> program is
                  required.
                </li>
                <li>
                  Requires <code>clang &ge; 14</code> on host, and backend
                  access to <code>/sys/fs/bpf</code>.
                </li>
                <li>
                  Tracepoint target format: <code>category/name</code>, e.g.
                  <code>syscalls/sys_enter_openat</code>.
                </li>
                <li>
                  Kprobe / Kretprobe targets are function names, e.g.
                  <code>do_unlinkat</code>.
                </li>
              </ul>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- --- Visual Builder --- -->
      <a-tab-pane key="visual">
        <template #tab
          ><span><ThunderboltOutlined /> Visual Builder</span></template
        >
        <PluginsVisualTab />
      </a-tab-pane>

      <!-- --- TS Pseudocode --- -->
      <a-tab-pane key="pseudo">
        <template #tab
          ><span><CodeOutlined /> TS Pseudocode</span></template
        >
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
