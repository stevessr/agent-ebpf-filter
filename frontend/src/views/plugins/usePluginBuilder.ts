import { ref, watch } from 'vue';
import { message } from 'ant-design-vue';
import type { PluginAttachKind, BPFTemplate } from '../../composables/plugins/usePlugins';

export interface BuilderState {
  id: string;
  name: string;
  description: string;
  author: string;
  version: string;
  attachKind: PluginAttachKind;
  attachTarget: string;
  programName: string;
  enabled: boolean;
  source: string;
}

const DEFAULT_BUILDER: BuilderState = {
  id: '',
  name: '',
  description: '',
  author: '',
  version: '1.0.0',
  attachKind: 'tracepoint',
  attachTarget: 'syscalls/sys_enter_execve',
  programName: 'trace_execve',
  enabled: false,
  source: '',
};

/**
 * Plugin builder state and operations.
 * Extracted from Plugins.vue.
 */
export function usePluginBuilder(
  compileBpf: (id: string, source: string) => Promise<boolean>,
  upsertPlugin: (payload: any) => Promise<any>,
  fetchPlugin: (id: string) => Promise<any>,
  compileLog: { value: string },
  lastCompile: { value: any },
  activeTab: { value: string },
) {
  const builder = ref<BuilderState>({ ...DEFAULT_BUILDER });
  const compiling = ref(false);
  const saving = ref(false);

  const attachKindOptions = [
    { value: 'tracepoint', label: 'Tracepoint' },
    { value: 'kprobe', label: 'Kprobe' },
    { value: 'kretprobe', label: 'Kretprobe' },
    { value: 'lsm', label: 'BPF LSM' },
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
    builder.value = { ...DEFAULT_BUILDER };
    compileLog.value = '';
    lastCompile.value = null;
  };

  const handleCompile = async () => {
    if (!builder.value.id) {
      message.warning('请填写插件 ID');
      return;
    }
    compiling.value = true;
    await compileBpf(builder.value.id, builder.value.source);
    compiling.value = false;
  };

  const handleSave = async () => {
    if (!builder.value.id || !builder.value.name) {
      message.warning('请填写 ID 与名称');
      return;
    }
    saving.value = true;
    const payload = {
      id: builder.value.id,
      name: builder.value.name,
      description: builder.value.description,
      author: builder.value.author,
      version: builder.value.version,
      kind: 'ebpf' as const,
      enabled: builder.value.enabled,
      attachKind: builder.value.attachKind,
      attachTarget: builder.value.attachTarget,
      programName: builder.value.programName,
      source: builder.value.source,
    };
    const saved = await upsertPlugin(payload);
    saving.value = false;
    if (saved) {
      activeTab.value = 'list';
    }
  };

  const handleLoadIntoBuilder = async (id: string) => {
    const data = await fetchPlugin(id);
    if (!data) return;
    const { plugin, source } = data;
    builder.value = {
      id: plugin.id,
      name: plugin.name,
      description: plugin.description || '',
      author: plugin.author || '',
      version: plugin.version || '1.0.0',
      attachKind: (plugin.attachKind || 'tracepoint') as PluginAttachKind,
      attachTarget: plugin.attachTarget || '',
      programName: plugin.programName || '',
      enabled: !!plugin.enabled,
      source,
    };
    activeTab.value = 'builder';
  };

  watch(
    () => builder.value.programName,
    (val) => {
      if (val && !builder.value.id) {
        builder.value.id = val.replace(/_/g, '-').toLowerCase();
      }
    },
  );

  return {
    builder,
    compiling,
    saving,
    attachKindOptions,
    applyTemplate,
    resetBuilder,
    handleCompile,
    handleSave,
    handleLoadIntoBuilder,
  };
}
