import { onMounted, ref } from "vue";
import axios from "axios";
import { backendError, renewToast } from "./notify";
import type { WrapperRule } from "../../types/config/registry";

export function useRenewRules() {
  const rules = ref<Record<string, WrapperRule>>({});
  const busy = ref(false);
  const ready = ref(false);
  const error = ref("");
  const comm = ref("");
  const action = ref("ALERT");
  const regex = ref("");
  const replacement = ref("");
  const priority = ref(0);
  const rewrite = ref("[]");
  const editing = ref("");
  const pendingDelete = ref("");
  async function load() {
    busy.value = true;
    error.value = "";
    try {
      rules.value = (await axios.get("/config/rules")).data;
      ready.value = true;
    } catch (cause) {
      error.value = backendError(cause, "规则请求失败");
      ready.value = false;
    } finally {
      busy.value = false;
    }
  }
  function edit(rule: WrapperRule) {
    editing.value = rule.comm;
    comm.value = rule.comm;
    action.value = rule.action;
    regex.value = rule.regex || "";
    replacement.value = rule.replacement || "";
    priority.value = rule.priority || 0;
    rewrite.value = JSON.stringify(rule.rewritten_cmd || []);
  }
  function reset() {
    editing.value = "";
    comm.value = "";
    regex.value = "";
    replacement.value = "";
    priority.value = 0;
    rewrite.value = "[]";
  }
  async function save() {
    if (busy.value || !ready.value || !comm.value.trim()) return;
    busy.value = true;
    error.value = "";
    try {
      const args =
        action.value === "REWRITE" && !regex.value
          ? JSON.parse(rewrite.value)
          : [];
      if (
        !Array.isArray(args) ||
        args.some((v) => typeof v !== "string") ||
        (action.value === "REWRITE" && !regex.value && !args.length)
      )
        throw new Error(
          '重写命令需要非空 JSON 字符串数组，如 ["echo","hello"]',
        );
      await axios.post("/config/rules", {
        comm: comm.value.trim(),
        action: action.value,
        regex: regex.value,
        replacement: replacement.value,
        priority: priority.value,
        rewritten_cmd: args,
      });
      renewToast.success(`规则已保存：${comm.value.trim()}`);
      reset();
      await load();
    } catch (cause) {
      error.value = backendError(cause, "规则请求失败");
    } finally {
      busy.value = false;
    }
  }
  async function remove() {
    if (busy.value || !pendingDelete.value) return;
    busy.value = true;
    error.value = "";
    try {
      await axios.delete(
        `/config/rules/${encodeURIComponent(pendingDelete.value)}`,
      );
      renewToast.success(`规则已删除：${pendingDelete.value}`);
      pendingDelete.value = "";
      await load();
    } catch (cause) {
      error.value = backendError(cause, "规则请求失败");
    } finally {
      busy.value = false;
    }
  }
  onMounted(() => void load());
  return {
    rules,
    busy,
    ready,
    error,
    comm,
    action,
    regex,
    replacement,
    priority,
    rewrite,
    editing,
    pendingDelete,
    load,
    edit,
    reset,
    save,
    remove,
  };
}
