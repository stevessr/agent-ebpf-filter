<script setup lang="ts">
import FileBrowserPanel from "../../explorer/FileBrowserPanel.vue";
import SanitizedFieldViewer from "../../common/SanitizedFieldViewer.vue";

import type {
  TLSBuiltinExecutableAttachStatus,
  TLSCaptureRule,
  TLSIgnoreRule,
  TLSLibraryStatus,
} from "../../../types/tls";
import {
  TLS_BUILTIN_COMMANDS,
  TLS_EXECUTABLE_LIBRARY_OPTIONS,
  TLS_MANUAL_HOOK_OPTIONS,
  TLS_RULE_SCOPE_OPTIONS,
  type TLSExecutableLibraryHint,
  type TLSManualHookType,
} from "../../../views/network/tlsCapture/constants";
import type { TLSExecutableAttachResult } from "../../../views/network/tlsCapture/types";
import {
  joinTLSRuleValues,
  updateTLSIgnoreRuleValues,
  updateTLSRuleValues,
  type TLSIgnoreRuleListField,
  type TLSRuleListField,
} from "../../../views/network/tlsCapture/utils";

interface FileEntry {
  name: string;
  isDir: boolean;
  path: string;
}

defineProps<{
  libraries: TLSLibraryStatus[];
  rulesLoading: boolean;
  ignoreRulesLoading: boolean;
  builtinAttachLoading: boolean;
  manualHookLoading: boolean;
  builtinAttachStatuses: TLSBuiltinExecutableAttachStatus[];
  executableAttachResult: TLSExecutableAttachResult | null;
}>();

const activeTab = defineModel<string>("activeTab", { required: true });
const rules = defineModel<TLSCaptureRule[]>("rules", { required: true });
const ignoreRules = defineModel<TLSIgnoreRule[]>("ignoreRules", {
  required: true,
});
const manualHookType = defineModel<TLSManualHookType>("manualHookType", {
  required: true,
});
const manualHookPid = defineModel<number | null>("manualHookPid", {
  required: true,
});
const executableLibraryHint = defineModel<TLSExecutableLibraryHint>(
  "executableLibraryHint",
  { required: true },
);
const executablePathInput = defineModel<string>("executablePathInput", {
  required: true,
});

const emit = defineEmits<{
  addRule: [];
  saveRules: [];
  removeRule: [id: string];
  attachBuiltins: [];
  attachBuiltinCommand: [command: string];
  attachExecutable: [];
  attachManualHook: [entry: FileEntry];
  addIgnoreRule: [];
  saveIgnoreRules: [];
  removeIgnoreRule: [id: string];
  persistIgnoreRules: [];
}>();

const manualHookOptions = TLS_MANUAL_HOOK_OPTIONS;
const executableLibraryOptions = TLS_EXECUTABLE_LIBRARY_OPTIONS;

const onRuleValuesChange = (
  rule: TLSCaptureRule,
  field: TLSRuleListField,
  event: Event,
) => {
  updateTLSRuleValues(rule, field, (event.target as HTMLInputElement).value);
  rules.value = [...rules.value];
};

const onIgnoreRuleValuesChange = (
  rule: TLSIgnoreRule,
  field: TLSIgnoreRuleListField,
  event: Event,
) => {
  updateTLSIgnoreRuleValues(
    rule,
    field,
    (event.target as HTMLInputElement).value,
  );
  ignoreRules.value = [...ignoreRules.value];
  emit("persistIgnoreRules");
};
</script>

<template>
  <a-card size="small" title="Hook SSL Management" class="tls-rules-card">
        <a-tabs v-model:activeKey="activeTab" size="small">
          <a-tab-pane key="rules" tab="Rules">
            <div class="tls-tab-actions">
              <a-space>
                <a-button size="small" @click="emit('addRule')">Add Rule</a-button>
                <a-button
                  size="small"
                  type="primary"
                  :loading="rulesLoading"
                  @click="emit('saveRules')"
                  >Save Rules</a-button
                >
              </a-space>
            </div>
            <a-list :data-source="rules" size="small" class="tls-rule-list">
              <template #renderItem="{ item }">
                <a-list-item class="tls-rule-item">
                  <div class="tls-rule-card">
                    <div class="tls-rule-header">
                      <a-space wrap>
                        <a-switch
                          v-model:checked="item.enabled"
                          checked-children="on"
                          un-checked-children="off"
                        />
                        <a-input
                          v-model:value="item.name"
                          size="small"
                          class="tls-rule-name"
                          placeholder="Rule name"
                        />
                        <a-select
                          v-model:value="item.scope"
                          size="small"
                          class="tls-rule-scope"
                          :options="TLS_RULE_SCOPE_OPTIONS"
                        />
                        <a-tag v-if="item.id === 'agent-cli-tag'" color="green"
                          >default</a-tag
                        >
                        <a-tag
                          v-else-if="item.scope === 'agent_cli_tag'"
                          color="cyan"
                          >agent context</a-tag
                        >
                      </a-space>
                      <a-button
                        v-if="item.id !== 'agent-cli-tag'"
                        size="small"
                        danger
                        ghost
                        @click="emit('removeRule', item.id)"
                        >Remove</a-button
                      >
                    </div>

                    <div class="tls-rule-fields">
                      <label class="tls-rule-field">
                        <span>Commands</span>
                        <a-input
                          size="small"
                          placeholder="claude, cursor, node"
                          :value="joinTLSRuleValues(item.comms)"
                          @change="onRuleValuesChange(item, 'comms', $event)"
                        />
                      </label>
                      <label class="tls-rule-field">
                        <span>Hosts</span>
                        <a-input
                          size="small"
                          placeholder="api.anthropic.com, github.com"
                          :value="joinTLSRuleValues(item.hosts)"
                          @change="onRuleValuesChange(item, 'hosts', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Methods</span>
                        <a-input
                          size="small"
                          placeholder="POST, GET"
                          :value="joinTLSRuleValues(item.methods)"
                          @change="onRuleValuesChange(item, 'methods', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Libraries</span>
                        <a-input
                          size="small"
                          placeholder="openssl, gnutls"
                          :value="joinTLSRuleValues(item.libraries)"
                          @change="
                            onRuleValuesChange(item, 'libraries', $event)
                          "
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Directions</span>
                        <a-input
                          size="small"
                          placeholder="send, recv"
                          :value="joinTLSRuleValues(item.directions)"
                          @change="
                            onRuleValuesChange(item, 'directions', $event)
                          "
                        />
                      </label>
                    </div>

                    <a-typography-text type="secondary" class="tls-rule-help">
                      {{
                        item.description ||
                        "All filled fields must match. Empty fields match any value."
                      }}
                    </a-typography-text>
                  </div>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>

          <a-tab-pane key="libraries" tab="Libraries">
            <a-list :data-source="libraries" size="small" bordered>
              <template #renderItem="{ item }">
                <a-list-item>
                  <a-list-item-meta :description="item.path || '—'">
                    <template #title>
                      <a-space>
                        <span>{{ item.name }}</span>
                        <a-tag :color="item.attached ? 'green' : 'default'">
                          {{ item.attached ? "Attached" : "Not attached" }}
                        </a-tag>
                        <a-tag v-if="item.available === false" color="red"
                          >Unavailable</a-tag
                        >
                      </a-space>
                    </template>
                  </a-list-item-meta>
                  <template #actions>
                    <span v-if="item.error" class="tls-error">{{
                      item.error
                    }}</span>
                  </template>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>

          <a-tab-pane key="manual" tab="Manual Hook">
            <a-alert
              type="info"
              show-icon
              class="tls-manual-hint"
              message="Select a local TLS library, Go binary, or executable hook target"
              description="Executable mode accepts a command name or binary path such as claude, /usr/local/bin/claude, node, deno, bun, codex, or a symlink/shebang CLI wrapper. The backend resolves symlinks and #! interpreters before attaching TLS uprobes."
            />
            <a-card size="small" class="tls-builtin-card">
              <template #title>Built-in SSL client binaries</template>
              <template #extra>
                <a-button
                  size="small"
                  type="primary"
                  :loading="builtinAttachLoading"
                  @click="emit('attachBuiltins')"
                  >Attach built-ins</a-button
                >
              </template>
              <a-space wrap>
                <a-button
                  v-for="command in TLS_BUILTIN_COMMANDS"
                  :key="command"
                  size="small"
                  @click="emit('attachBuiltinCommand', command)"
                >
                  {{ command }}
                </a-button>
              </a-space>
              <a-list
                v-if="builtinAttachStatuses.length"
                :data-source="builtinAttachStatuses"
                size="small"
                class="tls-builtin-status-list"
              >
                <template #renderItem="{ item }">
                  <a-list-item>
                    <a-list-item-meta
                      :title="`${item.target?.name || item.target?.command} (${item.target?.command})`"
                      :description="
                        item.result?.attachPath ||
                        item.result?.resolved?.realPath ||
                        item.error ||
                        item.target?.description
                      "
                    />
                    <template #actions>
                      <a-tag :color="item.attached ? 'green' : item.available ? 'orange' : 'red'">
                        {{ item.attached ? 'Attached' : item.available ? 'Available' : 'Missing' }}
                      </a-tag>
                    </template>
                  </a-list-item>
                </template>
              </a-list>
            </a-card>
            <a-space wrap class="tls-manual-controls">
              <span class="tls-manual-label">Target type</span>
              <a-select
                v-model:value="manualHookType"
                size="small"
                style="width: 190px"
                :options="manualHookOptions"
              />
              <template v-if="manualHookType === 'executable'">
                <span class="tls-manual-label">TLS symbols</span>
                <a-select
                  v-model:value="executableLibraryHint"
                  size="small"
                  style="width: 150px"
                  :options="executableLibraryOptions"
                />
              </template>
              <template
                v-if="
                  manualHookType === 'executable' || manualHookType === 'go'
                "
              >
                <span class="tls-manual-label">PID</span>
                <a-input-number
                  v-model:value="manualHookPid"
                  size="small"
                  :min="0"
                  placeholder="0 = all"
                  style="width: 120px"
                />
              </template>
              <a-tag v-if="manualHookLoading" color="blue">Attaching…</a-tag>
            </a-space>

            <a-input-search
              v-if="manualHookType === 'executable'"
              v-model:value="executablePathInput"
              class="tls-executable-input"
              placeholder="claude, /usr/local/bin/claude, /usr/bin/node, or /proc/<pid>/exe"
              enter-button="Hook executable"
              :loading="manualHookLoading"
              @search="emit('attachExecutable')"
            />

            <a-alert
              v-if="executableAttachResult"
              :type="executableAttachResult.error ? 'warning' : 'success'"
              show-icon
              class="tls-manual-hint"
              :message="
                executableAttachResult.error
                  ? 'Executable hook attach failed'
                  : 'Executable hook target resolved'
              "
            >
              <template #description>
                <a-descriptions size="small" :column="1" bordered>
                  <a-descriptions-item label="Input">
                    <SanitizedFieldViewer
                      :value="String(executableAttachResult.resolved?.input || executablePathInput || '—')"
                      :isSanitized="false"
                      field-name="input"
                    />
                  </a-descriptions-item>
                  <a-descriptions-item label="Resolved path">
                    <SanitizedFieldViewer
                      :value="String(executableAttachResult.resolved?.realPath || executableAttachResult.resolved?.path || '—')"
                      :isSanitized="false"
                      field-name="resolved path"
                    />
                  </a-descriptions-item>
                  <a-descriptions-item
                    v-if="executableAttachResult.resolved?.shebang"
                    label="Shebang"
                    >{{
                      executableAttachResult.resolved.shebang
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="Attach path">
                    <SanitizedFieldViewer
                      :value="String(executableAttachResult.attachPath || executableAttachResult.resolved?.realPath || '—')"
                      :isSanitized="false"
                      field-name="attach path"
                    />
                  </a-descriptions-item>
                  <a-descriptions-item label="Mode">{{
                    executableAttachResult.targetKind ||
                    executableAttachResult.library ||
                    "resolved"
                  }}</a-descriptions-item>
                </a-descriptions>
              </template>
            </a-alert>

            <FileBrowserPanel
              action-type="emit"
              action-label="Hook"
              :show-tracking-controls="false"
              :show-upload="false"
              :file-action-only="true"
              alert-message=""
              alert-description=""
              preview-title="TLS Hook File Preview"
              @action="emit('attachManualHook', $event)"
            />
          </a-tab-pane>
          <a-tab-pane key="ignore" tab="Ignore">
            <div class="tls-tab-actions">
              <a-space>
                <a-button size="small" @click="emit('addIgnoreRule')"
                  >Add Ignore Rule</a-button
                >
                <a-button
                  size="small"
                  type="primary"
                  :loading="ignoreRulesLoading"
                  @click="emit('saveIgnoreRules')"
                >Save Ignore Rules</a-button>
              </a-space>
            </div>
            <a-list :data-source="ignoreRules" size="small" class="tls-rule-list">
              <template #renderItem="{ item }">
                <a-list-item class="tls-rule-item">
                  <div class="tls-rule-card">
                    <div class="tls-rule-header">
                      <a-space wrap>
                        <a-switch
                          v-model:checked="item.enabled"
                          checked-children="on"
                          un-checked-children="off"
                          @change="emit('persistIgnoreRules')"
                        />
                        <a-input
                          v-model:value="item.name"
                          size="small"
                          class="tls-rule-name"
                          placeholder="Ignore rule name"
                        />
                      </a-space>
                      <a-button
                        size="small"
                        danger
                        ghost
                        @click="emit('removeIgnoreRule', item.id)"
                      >Remove</a-button>
                    </div>
                    <div class="tls-ignore-rule-fields">
                      <label class="tls-rule-field">
                        <span>Commands</span>
                        <a-input
                          size="small"
                          placeholder="node, claude, curl"
                          :value="joinTLSRuleValues(item.comms)"
                          @change="onIgnoreRuleValuesChange(item, 'comms', $event)"
                        />
                      </label>
                      <label class="tls-rule-field">
                        <span>Hosts</span>
                        <a-input
                          size="small"
                          placeholder="localhost, 127.0.0.1"
                          :value="joinTLSRuleValues(item.hosts)"
                          @change="onIgnoreRuleValuesChange(item, 'hosts', $event)"
                        />
                      </label>
                      <label class="tls-rule-field">
                        <span>URLs</span>
                        <a-input
                          size="small"
                          placeholder="/health, /ping"
                          :value="joinTLSRuleValues(item.urls)"
                          @change="onIgnoreRuleValuesChange(item, 'urls', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Methods</span>
                        <a-input
                          size="small"
                          placeholder="OPTIONS, GET"
                          :value="joinTLSRuleValues(item.methods)"
                          @change="onIgnoreRuleValuesChange(item, 'methods', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Libraries</span>
                        <a-input
                          size="small"
                          placeholder="openssl, gnutls"
                          :value="joinTLSRuleValues(item.libraries)"
                          @change="onIgnoreRuleValuesChange(item, 'libraries', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Directions</span>
                        <a-input
                          size="small"
                          placeholder="send, recv"
                          :value="joinTLSRuleValues(item.directions)"
                          @change="onIgnoreRuleValuesChange(item, 'directions', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Status codes</span>
                        <a-input
                          size="small"
                          placeholder="200, 404, 500"
                          :value="joinTLSRuleValues(item.statusCodes)"
                          @change="onIgnoreRuleValuesChange(item, 'statusCodes', $event)"
                        />
                      </label>
                    </div>
                    <a-typography-text type="secondary" class="tls-rule-help">
                      {{ item.description || "All filled fields must match to exclude an event. Empty fields are ignored." }}
                    </a-typography-text>
                  </div>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>
        </a-tabs>
  </a-card>
</template>

<style scoped>
.tls-rules-card {
  margin-bottom: 16px;
}

.tls-tab-actions {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}

.tls-manual-hint,
.tls-builtin-card,
.tls-manual-controls,
.tls-executable-input {
  margin-bottom: 12px;
}

.tls-builtin-status-list {
  margin-top: 10px;
}

.tls-manual-label {
  color: #64748b;
  font-size: 12px;
}

.tls-rule-list :deep(.ant-list-items) {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tls-rule-item {
  padding: 0 !important;
  border: 0 !important;
}

.tls-rule-card {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  background: #fff;
}

.tls-rule-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 12px;
}

.tls-rule-name {
  width: 260px;
}

.tls-rule-scope {
  width: 170px;
}

.tls-rule-fields {
  display: grid;
  grid-template-columns: minmax(220px, 1.4fr) minmax(260px, 1.6fr) repeat(
      3,
      minmax(140px, 1fr)
    );
  gap: 10px;
  align-items: end;
}

.tls-ignore-rule-fields {
  display: grid;
  grid-template-columns: repeat(4, minmax(160px, 1fr));
  gap: 10px;
  align-items: end;
}

.tls-rule-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #64748b;
  font-size: 12px;
}

.tls-rule-field span {
  line-height: 18px;
}

.tls-rule-help {
  display: block;
  margin-top: 10px;
}

.tls-error {
  color: #cf1322;
}

@media (max-width: 1200px) {
  .tls-rule-fields {
    grid-template-columns: repeat(2, minmax(220px, 1fr));
  }

  .tls-ignore-rule-fields {
    grid-template-columns: repeat(2, minmax(160px, 1fr));
  }
}

@media (max-width: 720px) {
  .tls-rule-header {
    flex-direction: column;
  }

  .tls-rule-name,
  .tls-rule-scope {
    width: 100%;
  }

  .tls-rule-fields,
  .tls-ignore-rule-fields {
    grid-template-columns: 1fr;
  }
}
</style>
