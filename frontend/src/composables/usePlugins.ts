import { ref } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';

export type PluginKind = 'ebpf' | 'webhook' | 'command';
export type PluginAttachKind = 'tracepoint' | 'kprobe' | 'kretprobe' | 'lsm' | 'none' | '';

export interface PluginManifest {
  id: string;
  name: string;
  description?: string;
  author?: string;
  version?: string;
  kind: PluginKind;
  enabled: boolean;
  createdAt?: string;
  updatedAt?: string;
  sourceSha256?: string;
  objectSha256?: string;
  attachKind?: PluginAttachKind;
  attachTarget?: string;
  programName?: string;
  webhookUrl?: string;
  webhookEvents?: string[];
  commandComm?: string;
  commandArgs?: string[];
  commandRule?: string;
  commandRewrite?: string[];
  loaded?: boolean;
  loadError?: string;
}

export interface BPFTemplate {
  id: string;
  name: string;
  description: string;
  attachKind: PluginAttachKind;
  attachTarget: string;
  programName: string;
  source: string;
}

export interface CompileResult {
  objectPath: string;
  sourceSha256: string;
  log: string;
  compiledAt: string;
}

export function usePlugins() {
  const plugins = ref<PluginManifest[]>([]);
  const templates = ref<BPFTemplate[]>([]);
  const currentSource = ref('');
  const compileLog = ref('');
  const lastCompile = ref<CompileResult | null>(null);
  const loading = ref(false);

  const fetchPlugins = async () => {
    loading.value = true;
    try {
      const res = await axios.get('/plugins');
      plugins.value = res.data.plugins || [];
    } catch (err) {
      message.error('加载插件列表失败');
    } finally {
      loading.value = false;
    }
  };

  const fetchTemplates = async () => {
    try {
      const res = await axios.get('/plugins/bpf/templates');
      templates.value = res.data.templates || [];
    } catch (err) {
      message.error('加载模板失败');
    }
  };

  const fetchPlugin = async (id: string): Promise<{ plugin: PluginManifest; source: string } | null> => {
    try {
      const res = await axios.get(`/plugins/${id}`);
      return { plugin: res.data.plugin, source: res.data.source || '' };
    } catch (err) {
      message.error('加载插件详情失败');
      return null;
    }
  };

  const upsertPlugin = async (payload: Partial<PluginManifest> & { id: string; source?: string }) => {
    try {
      const res = await axios.post('/plugins', payload);
      message.success(`插件 ${payload.id} 已保存`);
      await fetchPlugins();
      return res.data.plugin as PluginManifest;
    } catch (err: any) {
      message.error(`保存失败: ${err?.response?.data?.error || err?.message || ''}`);
      return null;
    }
  };

  const deletePlugin = async (id: string) => {
    try {
      await axios.delete(`/plugins/${id}`);
      message.success(`已删除 ${id}`);
      await fetchPlugins();
    } catch (err: any) {
      message.error(`删除失败: ${err?.response?.data?.error || ''}`);
    }
  };

  const togglePlugin = async (id: string, enabled: boolean) => {
    try {
      const res = await axios.post(`/plugins/${id}/toggle`, { enabled });
      const updated = res.data.plugin as PluginManifest;
      const idx = plugins.value.findIndex(p => p.id === id);
      if (idx >= 0) plugins.value[idx] = updated;
      message.success(enabled ? `已启用 ${id}` : `已停用 ${id}`);
      return updated;
    } catch (err: any) {
      message.error(`切换失败: ${err?.response?.data?.error || err?.message || ''}`);
      await fetchPlugins();
      return null;
    }
  };

  const compileBpf = async (id: string, source: string) => {
    compileLog.value = '';
    lastCompile.value = null;
    try {
      const res = await axios.post('/plugins/bpf/compile', { id, source });
      lastCompile.value = res.data as CompileResult;
      compileLog.value = res.data.log || '编译完成';
      message.success('编译成功');
      return true;
    } catch (err: any) {
      compileLog.value = err?.response?.data?.log || err?.response?.data?.error || err?.message || '';
      message.error(`编译失败: ${err?.response?.data?.error || err?.message || ''}`);
      return false;
    }
  };

  const loadBpf = async (id: string) => {
    try {
      await axios.post('/plugins/bpf/load', { id });
      message.success('加载成功');
      await fetchPlugins();
      return true;
    } catch (err: any) {
      message.error(`加载失败: ${err?.response?.data?.error || err?.message || ''}`);
      return false;
    }
  };

  const unloadBpf = async (id: string) => {
    try {
      await axios.post('/plugins/bpf/unload', { id });
      message.success('已卸载');
      await fetchPlugins();
      return true;
    } catch (err: any) {
      message.error(`卸载失败: ${err?.response?.data?.error || err?.message || ''}`);
      return false;
    }
  };

  return {
    plugins, templates, currentSource, compileLog, lastCompile, loading,
    fetchPlugins, fetchTemplates, fetchPlugin,
    upsertPlugin, deletePlugin, togglePlugin,
    compileBpf, loadBpf, unloadBpf,
  };
}
