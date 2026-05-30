import { computed } from 'vue';
import type { Ref } from 'vue';
import { Modal } from 'ant-design-vue';
import type { PluginManifest } from '../../composables/plugins/usePlugins';

/**
 * Plugin list display helpers.
 * Extracted from Plugins.vue.
 */
export function usePluginList(
  plugins: Ref<PluginManifest[]>,
  deletePlugin: (id: string) => Promise<unknown>,
  togglePlugin: (id: string, enabled: boolean) => Promise<unknown>,
) {
  const kindLabel = (kind: string) => {
    switch (kind) {
      case 'ebpf':
        return 'eBPF';
      case 'webhook':
        return 'Webhook';
      case 'command':
        return '命令规则';
      default:
        return kind;
    }
  };

  const statusTag = (plugin: PluginManifest) => {
    if (!plugin.enabled) return { color: 'default', text: '已禁用' };
    if (plugin.loadError) return { color: 'red', text: '加载失败' };
    if (plugin.loaded) return { color: 'green', text: '运行中' };
    return { color: 'orange', text: '已启用' };
  };

  const sortedPlugins = computed(() =>
    [...plugins.value].sort((a, b) => a.id.localeCompare(b.id)),
  );

  const confirmDelete = (plugin: PluginManifest) => {
    Modal.confirm({
      title: `删除插件 ${plugin.id}?`,
      content:
        '该操作会卸载已加载的 eBPF 程序并删除全部源码与编译产物。',
      okType: 'danger',
      onOk: () => deletePlugin(plugin.id),
    });
  };

  const handleToggle = async (plugin: PluginManifest, value: boolean) => {
    await togglePlugin(plugin.id, value);
  };

  return {
    kindLabel,
    statusTag,
    sortedPlugins,
    confirmDelete,
    handleToggle,
  };
}
