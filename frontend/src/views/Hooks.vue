<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import { LinkOutlined, CheckCircleOutlined, DeleteOutlined, ThunderboltOutlined, SwapOutlined, EditOutlined, PlusOutlined, CodeOutlined, FormOutlined } from '@ant-design/icons-vue';
import * as TOML from 'smol-toml';

import { getHookCliDoc, getHookEventDoc, type HookCliDoc, type HookEventDoc, type HookFieldDoc } from '../data/hookCatalog';

interface HookDef {
  id: string;
  name: string;
  description: string;
  target_cmd: string;
  hook_type: 'native' | 'wrapper';
  installed: boolean;
}

const hooks = ref<HookDef[]>([]);
const loading = ref(false);
// Track which hooks the user wants to force-use wrapper mode for.
const useWrapperOverride = ref<Record<string, boolean>>({});

// Configuration editor state
const showEditModal = ref(false);
const editingHook = ref<HookDef | null>(null);
const rawConfig = ref('');
const configPath = ref('');
const configFormat = ref<'json' | 'toml'>('json');
const savingConfig = ref(false);
const editorMode = ref<'visual' | 'raw'>('visual');
const newEventName = ref('');

// Parsed config for visual editing
const parsedConfig = ref<any>({});

const currentHookDoc = computed<HookCliDoc | null>(() => getHookCliDoc(editingHook.value?.id));
const documentedEventOptions = computed(() =>
  (currentHookDoc.value?.events || []).map((event) => ({
    label: event.name,
    value: event.name,
  })),
);
const selectedEventDoc = computed<HookEventDoc | null>(() =>
  getHookEventDoc(editingHook.value?.id, newEventName.value),
);
const usedEventNames = computed(() => new Set(Object.keys(parsedConfig.value?.hooks || {})));
const supportsAsyncCommandHooks = computed(() => editingHook.value?.id !== 'codex');
const supportsVisualEditor = computed(() => editingHook.value?.id !== 'kiro');

const normalizeParsedConfigForCurrentHook = () => {
  const normalized = JSON.parse(JSON.stringify(parsedConfig.value || {}));
  if (!normalized.hooks || editingHook.value?.id !== 'codex') {
    return normalized;
  }

  Object.values(normalized.hooks).forEach((matchers: any) => {
    if (!Array.isArray(matchers)) return;
    matchers.forEach((matcherBlock: any) => {
      if (!Array.isArray(matcherBlock?.hooks)) return;
      matcherBlock.hooks.forEach((hook: any) => {
        if (hook && typeof hook === 'object') {
          delete hook.async;
        }
      });
    });
  });

  return normalized;
};

const syncToParsed = () => {
  try {
    if (configFormat.value === 'toml') {
      parsedConfig.value = TOML.parse(rawConfig.value || '');
    } else {
      parsedConfig.value = JSON.parse(rawConfig.value || '{}');
    }
    if (!parsedConfig.value.hooks) parsedConfig.value.hooks = {};
  } catch (e) {
    console.error("Failed to parse config", e);
    parsedConfig.value = { hooks: {} };
  }
};

const syncToRaw = () => {
  try {
    const normalized = normalizeParsedConfigForCurrentHook();
    parsedConfig.value = normalized;
    if (configFormat.value === 'toml') {
      rawConfig.value = TOML.stringify(normalized);
    } else {
      rawConfig.value = JSON.stringify(normalized, null, 2);
    }
  } catch (e) {
    console.error("Failed to stringify parsed config", e);
  }
};

watch(editorMode, (newMode) => {
  if (newMode === 'visual') {
    syncToParsed();
  } else {
    syncToRaw();
  }
});

const fetchHooks = async () => {
  loading.value = true;
  try {
    const res = await axios.get('/config/hooks');
    hooks.value = res.data;
  } catch {
    message.error('Failed to fetch hooks');
  } finally {
    loading.value = false;
  }
};

const openEditModal = async (hook: HookDef) => {
  editingHook.value = hook;
  try {
    const res = await axios.get(`/config/hooks/${hook.id}/raw`);
    rawConfig.value = res.data.content;
    configPath.value = res.data.path;
    configFormat.value = res.data.format || 'json';
    editorMode.value = hook.id === 'kiro' ? 'raw' : 'visual';
    if (editorMode.value === 'visual') {
      syncToParsed();
    }
    showEditModal.value = true;
  } catch (err: any) {
    message.error(err.response?.data?.error || 'Failed to load configuration');
  }
};

const saveConfig = async () => {
  if (!editingHook.value) return;
  savingConfig.value = true;
  
  if (editorMode.value === 'visual') {
    syncToRaw();
  }
  
  try {
    // Validate format before saving
    if (configFormat.value === 'toml') {
      TOML.parse(rawConfig.value);
    } else {
      JSON.parse(rawConfig.value);
    }
    
    await axios.post(`/config/hooks/${editingHook.value.id}/raw`, {
      content: rawConfig.value
    });
    message.success('Configuration saved');
    showEditModal.value = false;
    await fetchHooks();
  } catch (err: any) {
    message.error(err.response?.data?.error || `Failed to save configuration. Ensure ${configFormat.value.toUpperCase()} is valid.`);
  } finally {
    savingConfig.value = false;
  }
};

const toggleHook = async (hook: HookDef) => {
  try {
    loading.value = true;
    await axios.post('/config/hooks', {
      id: hook.id,
      install: !hook.installed,
      use_wrapper: useWrapperOverride.value[hook.id] ?? false,
    });
    message.success(`${hook.installed ? 'Uninstalled' : 'Installed'} hook for ${hook.name}`);
    await fetchHooks();
  } catch {
    message.error(`Failed to ${hook.installed ? 'uninstall' : 'install'} hook`);
  } finally {
    loading.value = false;
  }
};

const addEvent = () => {
  const name = newEventName.value.trim();
  if (!name) {
    message.warning('Please select a hook event');
    return;
  }
  if (!parsedConfig.value.hooks) parsedConfig.value.hooks = {};
  if (parsedConfig.value.hooks[name]) {
    message.warning(`Event '${name}' already exists`);
    return;
  }
  parsedConfig.value.hooks[name] = [];
  newEventName.value = '';
};

const deleteEvent = (eventName: string) => {
  delete parsedConfig.value.hooks[eventName];
  message.success(`Deleted event: ${eventName}`);
};

const addMatcher = (eventName: string) => {
  parsedConfig.value.hooks[eventName].push({
    matcher: "*",
    hooks: []
  });
};

const deleteMatcher = (eventName: string, matcherIndex: number) => {
  parsedConfig.value.hooks[eventName].splice(matcherIndex, 1);
  message.success('Matcher block removed');
};

const addCommandHook = (eventName: string, matcherIndex: number) => {
  const nextHook: Record<string, unknown> = {
    type: "command",
    command: "",
    statusMessage: "Running hook...",
  };
  if (supportsAsyncCommandHooks.value) {
    nextHook.async = true;
  }
  parsedConfig.value.hooks[eventName][matcherIndex].hooks.push(nextHook);
};

const deleteCommandHook = (eventName: string, matcherIndex: number, hookIndex: number) => {
  parsedConfig.value.hooks[eventName][matcherIndex].hooks.splice(hookIndex, 1);
};

const getEventDoc = (eventName: string) => getHookEventDoc(editingHook.value?.id, eventName);
const getEventFields = (eventName: string) => getEventDoc(eventName)?.fields || [];
const getEventNotes = (eventName: string) => getEventDoc(eventName)?.notes || [];
const formatFieldLabel = (field: HookFieldDoc) => field.type ? `${field.name}: ${field.type}` : field.name;
const getHookDocSourceUrl = (hookId: string) => getHookCliDoc(hookId)?.sources?.[0]?.url || '';

onMounted(() => {

  void fetchHooks();
});
</script>

<template>
  <div style="background: #f0f2f5; padding: 24px; min-height: 100%;">
    <div style="max-width: 1000px; margin: 0 auto;">
      <a-card :bordered="false" style="border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.05);">
        <template #title>
          <div style="display: flex; align-items: center; gap: 8px;">
            <LinkOutlined style="color: #1890ff; font-size: 18px;" />
            <span style="font-size: 16px; font-weight: bold;">AI CLI Interception Hooks</span>
          </div>
        </template>
        <template #extra>
          <a-button @click="fetchHooks" :loading="loading" size="small">Refresh</a-button>
        </template>

        <a-alert
          message="Hook Modes"
          type="info"
          show-icon
          style="margin-bottom: 24px;"
        >
          <template #description>
            <div>
              <b>Native Hook</b> (recommended): Injects directly into the agent CLI's own config (e.g. Claude Code's <code>~/.claude/settings.json</code>). Intercepts every tool call with zero shell overhead.<br/>
              <b>Wrapper Hook</b>: Adds a shell alias so the CLI is transparently routed through <code>agent-wrapper</code>. Works for any CLI but requires a shell reload.
            </div>
          </template>
        </a-alert>

        <a-list
          :grid="{ gutter: 16, xs: 1, sm: 1, md: 2, lg: 2, xl: 2, xxl: 2 }"
          :dataSource="hooks"
          :loading="loading"
        >
          <template #renderItem="{ item }">
            <a-list-item>
              <a-card size="small" hoverable style="border-radius: 6px;">
                <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px;">
                  <div>
                    <h3 style="margin: 0; font-size: 15px; font-weight: 600;">{{ item.name }}</h3>
                    <div style="font-family: monospace; font-size: 12px; color: #888; margin-top: 4px;">
                      cmd: <span style="background: #f0f0f0; padding: 2px 6px; border-radius: 3px;">{{ item.target_cmd }}</span>
                    </div>
                  </div>
                  <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 4px;">
                    <a-tag :color="item.installed ? 'success' : 'default'">
                      <template #icon><CheckCircleOutlined v-if="item.installed" /></template>
                      {{ item.installed ? 'Installed' : 'Not Installed' }}
                    </a-tag>
                    <a-tag :color="item.hook_type === 'native' ? 'blue' : 'orange'">
                      <template #icon>
                        <ThunderboltOutlined v-if="item.hook_type === 'native'" />
                        <SwapOutlined v-else />
                      </template>
                      {{ item.hook_type === 'native' ? 'Native Hook' : 'Wrapper Hook' }}
                    </a-tag>
                  </div>
                </div>

                <p style="font-size: 13px; color: #555; min-height: 36px; margin-bottom: 12px;">
                  {{ item.description }}
                </p>

                <!-- For native-capable CLIs, allow opting into wrapper mode -->
                <div v-if="item.hook_type === 'native' && !item.installed" style="margin-bottom: 12px;">
                  <a-checkbox v-model:checked="useWrapperOverride[item.id]">
                    <span style="font-size: 12px; color: #888;">Use wrapper alias instead</span>
                  </a-checkbox>
                </div>

                <div style="text-align: right; border-top: 1px solid #f0f0f0; padding-top: 12px; display: flex; justify-content: flex-end; gap: 8px;">
                  <a-button
                    v-if="getHookDocSourceUrl(item.id)"
                    size="small"
                    :href="getHookDocSourceUrl(item.id)"
                    target="_blank"
                  >
                    <template #icon><LinkOutlined /></template>
                    Docs
                  </a-button>
                  <a-button
                    v-if="item.hook_type === 'native'"
                    size="small"
                    @click="openEditModal(item)"
                  >
                    <template #icon><EditOutlined /></template>
                    Edit Config
                  </a-button>
                  <a-button
                    :type="item.installed ? 'default' : 'primary'"
                    :danger="item.installed"
                    @click="toggleHook(item)"
                    :loading="loading"
                    size="small"
                  >
                    <template #icon>
                      <DeleteOutlined v-if="item.installed" />
                      <LinkOutlined v-else />
                    </template>
                    {{ item.installed ? 'Uninstall' : 'Install Hook' }}
                  </a-button>
                </div>
              </a-card>
            </a-list-item>
          </template>
        </a-list>
      </a-card>
    </div>

    <!-- Edit Configuration Modal -->
    <a-modal
      v-model:open="showEditModal"
      :title="`Edit Configuration: ${editingHook?.name}`"
      @ok="saveConfig"
      :confirmLoading="savingConfig"
      width="900px"
    >
      <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center;">
        <div>
          <span style="font-size: 12px; color: #888;">Config Path: </span>
          <a-typography-text code>{{ configPath }}</a-typography-text>
        </div>
        <a-radio-group v-if="supportsVisualEditor" v-model:value="editorMode" size="small">
          <a-radio-button value="visual"><FormOutlined /> Visual Editor</a-radio-button>
          <a-radio-button value="raw"><CodeOutlined /> Raw {{ configFormat.toUpperCase() }}</a-radio-button>
        </a-radio-group>
        <a-tag v-else color="gold">Raw editor only</a-tag>
      </div>

      <div v-if="editorMode === 'visual' && supportsVisualEditor" style="max-height: 60vh; overflow-y: auto; padding: 4px;">
        <a-alert v-if="currentHookDoc" type="info" show-icon style="margin-bottom: 16px;">
          <template #message>
            <span>Official docs for {{ currentHookDoc.name }}</span>
          </template>
          <template #description>
            <div style="display: flex; flex-direction: column; gap: 8px;">
              <div style="display: flex; gap: 12px; flex-wrap: wrap;">
                <a
                  v-for="source in currentHookDoc.sources"
                  :key="source.url"
                  :href="source.url"
                  target="_blank"
                  rel="noreferrer"
                >
                  {{ source.label }}
                </a>
              </div>

              <div v-if="currentHookDoc.commonFields?.length" style="display: flex; flex-wrap: wrap; gap: 6px;">
                <a-tag
                  v-for="field in currentHookDoc.commonFields"
                  :key="field.name"
                  color="blue"
                >
                  {{ formatFieldLabel(field) }}
                </a-tag>
              </div>

              <ul v-if="currentHookDoc.notes?.length" style="margin: 0; padding-left: 18px;">
                <li v-for="note in currentHookDoc.notes" :key="note">{{ note }}</li>
              </ul>
            </div>
          </template>
        </a-alert>

        <div v-if="Object.keys(parsedConfig.hooks || {}).length === 0" style="text-align: center; padding: 40px; color: #999;">
          No hooks configured. Click below to add an event.
        </div>
        
        <div v-for="(matchers, eventName) in (parsedConfig.hooks || {})" :key="eventName" style="margin-bottom: 24px; border: 1px solid #f0f0f0; border-radius: 8px; overflow: hidden;">
          <div style="background: #fafafa; padding: 8px 16px; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #f0f0f0;">
            <div style="display: flex; flex-direction: column; gap: 6px; min-width: 0;">
              <span style="font-weight: bold; color: #1890ff;">{{ eventName }}</span>
              <span v-if="getEventDoc(eventName as string)?.description" style="font-size: 12px; color: #666;">
                {{ getEventDoc(eventName as string)?.description }}
              </span>
              <div v-if="getEventFields(eventName as string).length" style="display: flex; flex-wrap: wrap; gap: 6px;">
                <a-tooltip v-for="field in getEventFields(eventName as string)" :key="field.name">
                  <template #title>{{ field.description }}</template>
                  <a-tag color="processing">{{ formatFieldLabel(field) }}</a-tag>
                </a-tooltip>
              </div>
              <ul v-if="getEventNotes(eventName as string).length" style="margin: 0; padding-left: 18px; color: #888; font-size: 12px;">
                <li v-for="note in getEventNotes(eventName as string)" :key="note">{{ note }}</li>
              </ul>
            </div>
            <div style="display: flex; gap: 8px;">
              <a-button size="small" @click="addMatcher(eventName as string)"><PlusOutlined /> Add Matcher</a-button>
              <a-popconfirm 
                :title="`Delete entire event '${eventName}'?`" 
                @confirm="deleteEvent(eventName as string)"
                ok-text="Yes"
                cancel-text="No"
              >
                <a-button size="small" danger ghost><DeleteOutlined /></a-button>
              </a-popconfirm>
            </div>
          </div>
          
          <div style="padding: 16px;">
            <div v-if="!matchers.length" style="text-align: center; color: #ccc; font-size: 12px;">No matchers defined</div>
            <div v-for="(matcherBlock, mIdx) in matchers" :key="mIdx" style="margin-bottom: 16px; padding: 12px; border: 1px dashed #e8e8e8; border-radius: 6px; position: relative;">
              <div style="margin-bottom: 12px; display: flex; align-items: center; gap: 8px;">
                <span style="font-size: 12px; font-weight: 500;">Matcher:</span>
                <a-input v-model:value="matcherBlock.matcher" size="small" placeholder="Tool name or * for all" style="width: 200px" />
                <a-popconfirm 
                  title="Delete this matcher block?" 
                  @confirm="deleteMatcher(eventName as string, Number(mIdx))"
                  ok-text="Yes"
                  cancel-text="No"
                >
                  <a-button size="small" type="link" danger style="position: absolute; right: 4px; top: 4px;">
                    <DeleteOutlined />
                  </a-button>
                </a-popconfirm>
              </div>

              <div style="margin-left: 20px;">
                <div v-for="(hook, hIdx) in matcherBlock.hooks" :key="hIdx" style="background: #fdfdfd; padding: 12px; border: 1px solid #f0f0f0; border-radius: 4px; margin-bottom: 8px; display: flex; flex-direction: column; gap: 8px; position: relative;">
                   <div style="display: flex; gap: 8px; align-items: center;">
                     <span style="font-size: 11px; width: 60px;">Command:</span>
                     <a-input v-model:value="hook.command" size="small" placeholder="Shell command" style="flex: 1" />
                     <a-popconfirm 
                       title="Delete this command hook?" 
                       @confirm="deleteCommandHook(eventName as string, Number(mIdx), Number(hIdx))"
                       ok-text="Yes"
                       cancel-text="No"
                     >
                       <a-button size="small" type="link" danger>
                         <DeleteOutlined />
                       </a-button>
                     </a-popconfirm>
                   </div>
                   <div style="display: flex; gap: 8px; align-items: center;">
                     <span style="font-size: 11px; width: 60px;">Message:</span>
                     <a-input v-model:value="hook.statusMessage" size="small" placeholder="Display message" style="flex: 1" />
                     <template v-if="supportsAsyncCommandHooks">
                       <span style="font-size: 11px; margin-left: 12px;">Async:</span>
                       <a-switch v-model:checked="hook.async" size="small" />
                     </template>
                   </div>
                </div>
                <a-button size="small" type="dashed" block @click="addCommandHook(eventName as string, Number(mIdx))">
                  <PlusOutlined /> Add Command Hook
                </a-button>
              </div>
            </div>
          </div>
        </div>
        
        <div style="margin-top: 16px; background: #fafafa; padding: 12px; border: 1px dashed #d9d9d9; border-radius: 8px;">
          <div style="display: flex; gap: 8px; align-items: center; flex-wrap: wrap;">
            <span style="font-size: 12px; color: #888;">New Event:</span>
            <a-select
              v-model:value="newEventName"
              size="small"
              show-search
              :options="documentedEventOptions"
              placeholder="Select official hook event"
              style="width: 260px"
              option-filter-prop="label"
            />
            <a-button type="primary" size="small" @click="addEvent" :disabled="!newEventName || usedEventNames.has(newEventName)">
              <PlusOutlined /> Add Hook Event
            </a-button>
            <span v-if="newEventName && usedEventNames.has(newEventName)" style="font-size: 12px; color: #fa8c16;">
              This event already exists in the current config.
            </span>
          </div>

          <div v-if="selectedEventDoc" style="margin-top: 12px; display: flex; flex-direction: column; gap: 8px;">
            <div style="font-size: 12px; color: #666;">{{ selectedEventDoc.description }}</div>
            <div v-if="selectedEventDoc.matcher" style="font-size: 12px; color: #888;">
              Matcher filters: {{ selectedEventDoc.matcher }}
            </div>
            <div v-if="selectedEventDoc.fields?.length" style="display: flex; flex-wrap: wrap; gap: 6px;">
              <a-tooltip v-for="field in selectedEventDoc.fields" :key="field.name">
                <template #title>{{ field.description }}</template>
                <a-tag color="processing">{{ formatFieldLabel(field) }}</a-tag>
              </a-tooltip>
            </div>
            <ul v-if="selectedEventDoc.notes?.length" style="margin: 0; padding-left: 18px; color: #888; font-size: 12px;">
              <li v-for="note in selectedEventDoc.notes" :key="note">{{ note }}</li>
            </ul>
          </div>
        </div>
      </div>

      <div v-else>
        <a-alert
          v-if="editingHook?.id === 'kiro'"
          type="info"
          show-icon
          style="margin-bottom: 12px;"
          message="Kiro uses agent-scoped native hook JSON"
          description="This managed Kiro agent is edited in raw mode because Kiro's hook schema differs from the generic visual editor used for Claude / Gemini / Codex / Copilot."
        />
        <a-textarea
          v-model:value="rawConfig"
          :rows="20"
          style="font-family: monospace; font-size: 12px; background: #fafafa;"
          placeholder="{ ... }"
        />
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
code {
  background: #f0f0f0;
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 12px;
}
</style>
