<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import {
  DeleteOutlined,
  PlusOutlined,
  CodeOutlined,
  FormOutlined,
} from '@ant-design/icons-vue';
import * as TOML from 'smol-toml';

import {
  getHookCliDoc,
  getHookEventDoc,
  type HookCliDoc,
  type HookEventDoc,
  type HookFieldDoc,
} from '../../data/hookCatalog';
import type { HookDef } from '../../types/hooks';

const props = defineProps<{
  open: boolean;
  hook: HookDef | null;
}>();

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void;
  (e: 'saved'): void;
}>();

const localOpen = computed({
  get: () => props.open,
  set: (value) => emit('update:open', value),
});

const rawConfig = ref('');
const configPath = ref('');
const configFormat = ref<'json' | 'toml'>('json');
const savingConfig = ref(false);
const editorMode = ref<'visual' | 'raw'>('visual');
const newEventName = ref('');
const parsedConfig = ref<any>({ hooks: {} });

const currentHookDoc = computed<HookCliDoc | null>(() => getHookCliDoc(props.hook?.id));
const documentedEventOptions = computed(() =>
  (currentHookDoc.value?.events || []).map((event) => ({
    label: event.name,
    value: event.name,
  })),
);
const selectedEventDoc = computed<HookEventDoc | null>(() =>
  getHookEventDoc(props.hook?.id, newEventName.value),
);
const usedEventNames = computed(() => new Set(Object.keys(parsedConfig.value?.hooks || {})));
const supportsAsyncCommandHooks = computed(
  () => props.hook?.id !== 'codex' && props.hook?.id !== 'augment',
);
const supportsTimeoutCommandHooks = computed(() => props.hook?.id === 'augment');
const supportsVisualEditor = computed(() => props.hook?.id !== 'kiro');

const stripUnsupportedFields = (cfg: any): any => {
  const normalized = JSON.parse(JSON.stringify(cfg || {}));
  if (!normalized.hooks) return normalized;
  const id = props.hook?.id;
  if (id !== 'codex' && id !== 'augment') return normalized;
  Object.values(normalized.hooks).forEach((matchers: any) => {
    if (!Array.isArray(matchers)) return;
    matchers.forEach((matcherBlock: any) => {
      if (!Array.isArray(matcherBlock?.hooks)) return;
      matcherBlock.hooks.forEach((hook: any) => {
        if (hook && typeof hook === 'object') delete hook.async;
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
    console.error('Failed to parse config', e);
    parsedConfig.value = { hooks: {} };
  }
};

const syncToRaw = () => {
  try {
    const normalized = stripUnsupportedFields(parsedConfig.value);
    parsedConfig.value = normalized;
    rawConfig.value =
      configFormat.value === 'toml' ? TOML.stringify(normalized) : JSON.stringify(normalized, null, 2);
  } catch (e) {
    console.error('Failed to stringify parsed config', e);
  }
};

watch(editorMode, (newMode) => {
  if (newMode === 'visual') syncToParsed();
  else syncToRaw();
});

const loadConfig = async () => {
  if (!props.hook) return;
  try {
    const res = await axios.get(`/config/hooks/${props.hook.id}/raw`);
    rawConfig.value = res.data.content;
    configPath.value = res.data.path;
    configFormat.value = res.data.format || 'json';
    editorMode.value = props.hook.id === 'kiro' ? 'raw' : 'visual';
    if (editorMode.value === 'visual') syncToParsed();
  } catch (err: any) {
    message.error(err.response?.data?.error || 'Failed to load configuration');
    emit('update:open', false);
  }
};

watch(
  () => [props.open, props.hook?.id],
  ([open]) => {
    if (open && props.hook) void loadConfig();
  },
);

const saveConfig = async () => {
  if (!props.hook) return;
  savingConfig.value = true;
  if (editorMode.value === 'visual') syncToRaw();
  try {
    if (configFormat.value === 'toml') TOML.parse(rawConfig.value);
    else JSON.parse(rawConfig.value);
    await axios.post(`/config/hooks/${props.hook.id}/raw`, { content: rawConfig.value });
    message.success('Configuration saved');
    emit('update:open', false);
    emit('saved');
  } catch (err: any) {
    message.error(
      err.response?.data?.error ||
        `Failed to save configuration. Ensure ${configFormat.value.toUpperCase()} is valid.`,
    );
  } finally {
    savingConfig.value = false;
  }
};

const addEvent = () => {
  const name = newEventName.value.trim();
  if (!name) return message.warning('Please select a hook event');
  if (!parsedConfig.value.hooks) parsedConfig.value.hooks = {};
  if (parsedConfig.value.hooks[name]) return message.warning(`Event '${name}' already exists`);
  parsedConfig.value.hooks[name] = [];
  newEventName.value = '';
};
const deleteEvent = (eventName: string) => {
  delete parsedConfig.value.hooks[eventName];
  message.success(`Deleted event: ${eventName}`);
};
const addMatcher = (eventName: string) => {
  parsedConfig.value.hooks[eventName].push({ matcher: '*', hooks: [] });
};
const deleteMatcher = (eventName: string, matcherIndex: number) => {
  parsedConfig.value.hooks[eventName].splice(matcherIndex, 1);
  message.success('Matcher block removed');
};
const addCommandHook = (eventName: string, matcherIndex: number) => {
  const nextHook: Record<string, unknown> = {
    type: 'command',
    command: '',
    statusMessage: 'Running hook...',
  };
  if (supportsAsyncCommandHooks.value) nextHook.async = true;
  if (supportsTimeoutCommandHooks.value) nextHook.timeout = 5000;
  parsedConfig.value.hooks[eventName][matcherIndex].hooks.push(nextHook);
};
const deleteCommandHook = (eventName: string, matcherIndex: number, hookIndex: number) => {
  parsedConfig.value.hooks[eventName][matcherIndex].hooks.splice(hookIndex, 1);
};

const getEventDoc = (eventName: string) => getHookEventDoc(props.hook?.id, eventName);
const getEventFields = (eventName: string) => getEventDoc(eventName)?.fields || [];
const getEventNotes = (eventName: string) => getEventDoc(eventName)?.notes || [];
const formatFieldLabel = (field: HookFieldDoc) =>
  field.type ? `${field.name}: ${field.type}` : field.name;
</script>

<template>
  <a-modal
    v-model:open="localOpen"
    :title="`Edit Configuration: ${hook?.name ?? ''}`"
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
              <a v-for="source in currentHookDoc.sources" :key="source.url" :href="source.url" target="_blank" rel="noreferrer">
                {{ source.label }}
              </a>
            </div>
            <div v-if="currentHookDoc.commonFields?.length" style="display: flex; flex-wrap: wrap; gap: 6px;">
              <a-tag v-for="field in currentHookDoc.commonFields" :key="field.name" color="blue">
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
            <a-popconfirm :title="`Delete entire event '${eventName}'?`" @confirm="deleteEvent(eventName as string)" ok-text="Yes" cancel-text="No">
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
              <a-popconfirm title="Delete this matcher block?" @confirm="deleteMatcher(eventName as string, Number(mIdx))" ok-text="Yes" cancel-text="No">
                <a-button size="small" type="link" danger style="position: absolute; right: 4px; top: 4px;">
                  <DeleteOutlined />
                </a-button>
              </a-popconfirm>
            </div>

            <div style="margin-left: 20px;">
              <div v-for="(hookEntry, hIdx) in matcherBlock.hooks" :key="hIdx" style="background: #fdfdfd; padding: 12px; border: 1px solid #f0f0f0; border-radius: 4px; margin-bottom: 8px; display: flex; flex-direction: column; gap: 8px; position: relative;">
                <div style="display: flex; gap: 8px; align-items: center;">
                  <span style="font-size: 11px; width: 60px;">Command:</span>
                  <a-input v-model:value="hookEntry.command" size="small" placeholder="Shell command" style="flex: 1" />
                  <a-popconfirm title="Delete this command hook?" @confirm="deleteCommandHook(eventName as string, Number(mIdx), Number(hIdx))" ok-text="Yes" cancel-text="No">
                    <a-button size="small" type="link" danger>
                      <DeleteOutlined />
                    </a-button>
                  </a-popconfirm>
                </div>
                <div style="display: flex; gap: 8px; align-items: center;">
                  <span style="font-size: 11px; width: 60px;">Message:</span>
                  <a-input v-model:value="hookEntry.statusMessage" size="small" placeholder="Display message" style="flex: 1" />
                  <template v-if="supportsAsyncCommandHooks">
                    <span style="font-size: 11px; margin-left: 12px;">Async:</span>
                    <a-switch v-model:checked="hookEntry.async" size="small" />
                  </template>
                  <template v-if="supportsTimeoutCommandHooks">
                    <span style="font-size: 11px; margin-left: 12px;">Timeout (ms):</span>
                    <a-input-number v-model:value="hookEntry.timeout" size="small" :min="0" :step="1000" style="width: 100px" />
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
        v-if="hook?.id === 'kiro'"
        type="info"
        show-icon
        style="margin-bottom: 12px;"
        message="Kiro uses agent-scoped native hook JSON"
        description="This managed Kiro agent is edited in raw mode because Kiro's hook schema differs from the generic visual editor used for Claude / Gemini / Codex / Copilot / Augment."
      />
      <a-textarea
        v-model:value="rawConfig"
        :rows="20"
        style="font-family: monospace; font-size: 12px; background: #fafafa;"
        placeholder="{ ... }"
      />
    </div>
  </a-modal>
</template>

