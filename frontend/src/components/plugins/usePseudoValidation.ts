import { computed } from 'vue';
import type { Ref } from 'vue';
import type { VisualWorkspaceSnapshot } from './types';
import { getAttachKindForTrigger, getAttachTargetForTrigger, PSEUDO_PROGRAM_NAME } from './trigger-runtime';
import { countConditions } from './validation';

interface ValidationIssue {
  severity: 'error' | 'warning' | 'info';
  text: string;
}

/**
 * Validation logic for the TS pseudocode workspace.
 * Extracted from PluginsPseudoCodeTab.vue.
 */
export function usePseudoValidation(
  pluginId: Ref<string>,
  pseudoCode: Ref<string>,
  parsedSnapshot: Ref<VisualWorkspaceSnapshot>,
) {
  const conditionCount = computed(() =>
    countConditions(parsedSnapshot.value.conditions),
  );

  const attachKind = computed(() =>
    getAttachKindForTrigger(parsedSnapshot.value.trigger),
  );

  const attachTarget = computed(() =>
    getAttachTargetForTrigger(parsedSnapshot.value.trigger),
  );

  const validationIssues = computed(() => {
    const issues: ValidationIssue[] = [];
    if (!/^[a-z0-9][a-z0-9-]{2,63}$/.test(pluginId.value.trim())) {
      issues.push({
        severity: 'error',
        text: '插件 ID 必须为 3-64 位小写字母、数字或中划线，且以字母/数字开头。',
      });
    }
    if (
      !pseudoCode.value.includes('export default function filter')
    ) {
      issues.push({
        severity: 'error',
        text: 'TS 伪代码必须包含 export default function filter(ctx: HookContext) 入口。',
      });
    }
    if (!/if\s*\(/.test(pseudoCode.value)) {
      issues.push({
        severity: 'error',
        text: '当前独立编译器需要至少一个 if (...) 条件作为过滤边界。',
      });
    }
    if (!/Action\.\w+\s*\(/.test(pseudoCode.value)) {
      issues.push({
        severity: 'error',
        text: '请在命中条件内调用 Action.block() / Action.alert() / Action.kill()。',
      });
    }
    if (
      parsedSnapshot.value.trigger === 'unlink' &&
      parsedSnapshot.value.action === 'BLOCK'
    ) {
      issues.push({
        severity: 'error',
        text: 'unlink 走 kprobe/do_unlinkat，不能直接 BLOCK，请改用 Action.alert() 或 Action.kill()。',
      });
    }
    if (conditionCount.value > 8) {
      issues.push({
        severity: 'error',
        text: '解析出的条件超过 8 个，容易触发 eBPF verifier 复杂度上限。',
      });
    }
    if (parsedSnapshot.value.mapMode === 'BLOCKLIST') {
      issues.push({
        severity: 'warning',
        text: 'BLOCKLIST 只生成查表逻辑，仍需运行时写入对应 map key。',
      });
    }
    return issues;
  });

  const validationErrors = computed(() =>
    validationIssues.value.filter((issue) => issue.severity === 'error'),
  );

  const compileReady = computed(() => validationErrors.value.length === 0);

  return {
    conditionCount,
    attachKind,
    attachTarget,
    validationIssues,
    validationErrors,
    compileReady,
    PSEUDO_PROGRAM_NAME,
  };
}
