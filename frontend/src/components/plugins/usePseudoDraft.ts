import { ref, nextTick } from 'vue';
import type { Ref } from 'vue';
import { message } from 'ant-design-vue';

export interface PseudoDraft {
  version: 1;
  pluginId: string;
  pluginName: string;
  description: string;
  pseudoCode: string;
}

const DEFAULT_PSEUDO_CODE = `import { process, Action, Maps, HookContext } from "ebpf";

export default function filter(ctx: HookContext) {
  // TS 伪代码工作区是独立的，不会回写或读取可视化画布。
  if (ctx.comm === "nc") {
    Action.block();
  }
}
`;

const PSEUDO_STORAGE_KEY = 'agent-ebpf-filter.ts-pseudocode.workspace.v1';

/**
 * Draft persistence for the TS pseudocode workspace.
 * Extracted from PluginsPseudoCodeTab.vue.
 */
export function usePseudoDraft(
  pluginId: Ref<string>,
  pluginName: Ref<string>,
  description: Ref<string>,
  pseudoCode: Ref<string>,
  compiled: Ref<boolean>,
  compileLogLocal: Ref<string>,
  editorRef: { value: { layout(): void } | null },
) {
  const autosaveLabel = ref('独立 TS 草稿未加载');

  const canUseLocalStorage = () =>
    typeof window !== 'undefined' &&
    typeof window.localStorage !== 'undefined';

  const getTimeLabel = () => new Date().toLocaleTimeString();

  const createDraft = (): PseudoDraft => ({
    version: 1,
    pluginId: pluginId.value,
    pluginName: pluginName.value,
    description: description.value,
    pseudoCode: pseudoCode.value,
  });

  const applyDraft = (draft: Partial<PseudoDraft>) => {
    if (draft.pluginId) pluginId.value = draft.pluginId;
    if (draft.pluginName) pluginName.value = draft.pluginName;
    if (draft.description) description.value = draft.description;
    if (typeof draft.pseudoCode === 'string')
      pseudoCode.value = draft.pseudoCode;
  };

  const saveDraft = (silent = false) => {
    if (!canUseLocalStorage()) {
      autosaveLabel.value = '当前环境不支持 localStorage 草稿';
      return;
    }
    window.localStorage.setItem(
      PSEUDO_STORAGE_KEY,
      JSON.stringify(createDraft()),
    );
    autosaveLabel.value = `TS 草稿已保存 ${getTimeLabel()}`;
    if (!silent)
      message.success('TS 伪代码草稿已保存到独立浏览器存储槽');
  };

  const restoreDraft = () => {
    if (!canUseLocalStorage()) return;
    const raw = window.localStorage.getItem(PSEUDO_STORAGE_KEY);
    if (!raw) {
      autosaveLabel.value = '尚无独立 TS 伪代码草稿';
      return;
    }
    try {
      applyDraft(JSON.parse(raw) as Partial<PseudoDraft>);
      autosaveLabel.value = `已恢复 TS 草稿 ${getTimeLabel()}`;
      message.info('已恢复独立 TS 伪代码草稿');
    } catch (err) {
      autosaveLabel.value = 'TS 草稿损坏，已忽略';
      console.warn('Failed to restore TS pseudocode draft:', err);
    }
  };

  const clearDraft = () => {
    if (!canUseLocalStorage()) return;
    window.localStorage.removeItem(PSEUDO_STORAGE_KEY);
    autosaveLabel.value =
      'TS 草稿已清除；继续编辑会自动重新保存';
    message.success('已清除独立 TS 伪代码存储槽');
  };

  const resetDraft = () => {
    pluginId.value = 'ts-pseudocode-filter';
    pluginName.value = 'TS 伪代码过滤插件';
    description.value =
      '由独立 TS 伪代码工作区生成的 eBPF 过滤审计插件。';
    pseudoCode.value = DEFAULT_PSEUDO_CODE;
    compiled.value = false;
    compileLogLocal.value = '';
    void nextTick(() => editorRef.value?.layout());
  };

  return {
    autosaveLabel,
    pseudoStorageKey: PSEUDO_STORAGE_KEY,
    defaultPseudoCode: DEFAULT_PSEUDO_CODE,
    saveDraft,
    restoreDraft,
    clearDraft,
    resetDraft,
  };
}
