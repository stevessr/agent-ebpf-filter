import { ref } from "vue";
import { message } from "ant-design-vue";
import { usePlugins } from "../../composables/usePlugins";
import type {
  VisualTrigger,
  VisualValidationIssue,
} from "./types";
import { getAttachKindForTrigger, getAttachTargetForTrigger, VISUAL_PROGRAM_NAME } from "./trigger-runtime";

export interface UsePluginCompilerOptions {
  pluginId: () => string;
  pluginName: () => string;
  description: () => string;
  trigger: () => VisualTrigger;
  generatedBpfCode: () => string;
  isWorkspaceValid: () => boolean;
  validationErrors: () => VisualValidationIssue[];
  isCompiled: { value: boolean };
}

export function usePluginCompiler(opts: UsePluginCompilerOptions) {
  const { compileBpf, loadBpf, upsertPlugin, fetchPlugins } = usePlugins();

  const compiling = ref(false);
  const loadingAction = ref(false);
  const compileLogLocal = ref("");

  const visualAttachKind = () => getAttachKindForTrigger(opts.trigger());
  const visualAttachTarget = () => getAttachTargetForTrigger(opts.trigger());

  const handleCompileAndRegister = async () => {
    if (!opts.isWorkspaceValid()) {
      compileLogLocal.value = [
        "已阻止编译：当前积木工作台存在错误。",
        ...opts.validationErrors().map(
          (issue) =>
            `[${issue.severity.toUpperCase()}] ${issue.title}${
              issue.detail ? ` - ${issue.detail}` : ""
            }`
        ),
      ].join("\n");
      message.error(`请先修复左侧"编译前验证"中的错误`);
      return;
    }
    compiling.value = true;
    compileLogLocal.value = "正在将高阶规则积木块转译为标准的 BPF C 源码...\n";
    try {
      compileLogLocal.value += `正在注册插件 Manifest [${opts.pluginId()}] 至本地仓库...\n`;
      compileLogLocal.value += `挂载方式: ${visualAttachKind()} / ${visualAttachTarget()} / program=${VISUAL_PROGRAM_NAME}\n`;
      await upsertPlugin({
        id: opts.pluginId(),
        name: opts.pluginName(),
        description: opts.description(),
        kind: "ebpf",
        enabled: false,
        attachKind: visualAttachKind(),
        attachTarget: visualAttachTarget(),
        programName: VISUAL_PROGRAM_NAME,
        source: opts.generatedBpfCode(),
      });

      compileLogLocal.value +=
        "正在调用 LLVM/Clang 将源码编译为 ELF 内核字节码...\n";
      const success = await compileBpf(opts.pluginId(), opts.generatedBpfCode());
      if (success) {
        opts.isCompiled.value = true;
        compileLogLocal.value +=
          "\n[SUCCESS] 编译成功！点击下方按钮即可一键挂载至内核运行生效。";
      } else {
        compileLogLocal.value +=
          "\n[ERROR] 编译失败，请排查过滤表达式是否在内核 Verifier 安全范围内。";
      }
    } catch (err: any) {
      compileLogLocal.value += `\n[ERROR] 错误: ${err.message}`;
    } finally {
      compiling.value = false;
    }
  };

  const handleLoad = async () => {
    loadingAction.value = true;
    try {
      await loadBpf(opts.pluginId());
      await fetchPlugins();
    } finally {
      loadingAction.value = false;
    }
  };

  return {
    compiling,
    loadingAction,
    compileLogLocal,
    handleCompileAndRegister,
    handleLoad,
  };
}
