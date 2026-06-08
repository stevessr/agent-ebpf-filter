import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import type { RedactionLevel, RedactionPolicy, RedactionRule } from "../../../backend/redaction/types";

type RedactionPolicyResponse = {
  redactionPolicy?: RedactionPolicy;
  redaction_policy?: RedactionPolicy;
  policy?: RedactionPolicy;
};

const DEFAULT_POLICY: RedactionPolicy = {
  level: "standard",
  rules: [],
  defaultPlaceholder: "[REDACTED]",
  preserveLengths: false,
  excludeCategories: [],
};

const normalizePolicy = (input?: Partial<RedactionPolicy> | null): RedactionPolicy => ({
  level: (input?.level as RedactionLevel) || DEFAULT_POLICY.level,
  rules: Array.isArray(input?.rules) ? (input.rules as RedactionRule[]) : [],
  defaultPlaceholder: input?.defaultPlaceholder || DEFAULT_POLICY.defaultPlaceholder,
  preserveLengths: Boolean(input?.preserveLengths),
  excludeCategories: Array.isArray(input?.excludeCategories)
    ? input.excludeCategories.slice()
    : [],
});

const extractPolicy = (data: RedactionPolicyResponse | RedactionPolicy | null | undefined) => {
  if (!data) return null;
  if ("level" in data) return data as RedactionPolicy;
  return data.redactionPolicy || data.redaction_policy || data.policy || null;
};

export function useRedactionPolicy() {
  const policy = ref<RedactionPolicy>(normalizePolicy(DEFAULT_POLICY));
  const loading = ref(false);
  const saving = ref(false);
  const lastUpdatedAt = ref<string>("");
  const pollTimer = ref<number | null>(null);
  const dirty = ref(false);

  const level = computed({
    get: () => policy.value.level,
    set: (value: RedactionLevel) => {
      policy.value.level = value;
      dirty.value = true;
    },
  });

  const rules = computed({
    get: () => policy.value.rules,
    set: (value: RedactionRule[]) => {
      policy.value.rules = Array.isArray(value) ? value : [];
      dirty.value = true;
    },
  });

  const applyPolicy = (next: Partial<RedactionPolicy>) => {
    policy.value = normalizePolicy(next);
    dirty.value = false;
    lastUpdatedAt.value = new Date().toISOString();
  };

  const fetchPolicy = async () => {
    loading.value = true;
    try {
      const res = await axios.get("/config/redaction-policy");
      const next = extractPolicy(res.data);
      if (next) {
        policy.value = normalizePolicy(next);
        dirty.value = false;
        lastUpdatedAt.value = new Date().toISOString();
      }
    } catch (err: any) {
      message.error(err?.response?.data?.error || "Failed to load redaction policy");
    } finally {
      loading.value = false;
    }
  };

  const savePolicy = async () => {
    saving.value = true;
    try {
      const res = await axios.put("/config/redaction-policy", policy.value);
      const next = extractPolicy(res.data);
      if (next) {
        policy.value = normalizePolicy(next);
      }
      dirty.value = false;
      lastUpdatedAt.value = new Date().toISOString();
      message.success("Redaction policy saved");
    } catch (err: any) {
      message.error(err?.response?.data?.error || "Failed to save redaction policy");
    } finally {
      saving.value = false;
    }
  };

  const setLevel = (value: RedactionLevel) => {
    level.value = value;
  };

  const updateRules = (nextRules: RedactionRule[]) => {
    rules.value = nextRules;
  };

  const syncPolicy = async () => {
    try {
      const res = await axios.get("/config/redaction-policy");
      const next = extractPolicy(res.data);
      if (!next) return;
      const serializedCurrent = JSON.stringify(policy.value);
      const serializedNext = JSON.stringify(normalizePolicy(next));
      if (serializedCurrent !== serializedNext) {
        policy.value = normalizePolicy(next);
        dirty.value = false;
        lastUpdatedAt.value = new Date().toISOString();
      }
    } catch (_) {
      // background sync only
    }
  };

  const startListening = () => {
    if (pollTimer.value != null) return;
    pollTimer.value = window.setInterval(() => {
      void syncPolicy();
    }, 15000);
  };

  const stopListening = () => {
    if (pollTimer.value != null) {
      window.clearInterval(pollTimer.value);
      pollTimer.value = null;
    }
  };

  onMounted(() => {
    void fetchPolicy();
    startListening();
  });

  onBeforeUnmount(() => {
    stopListening();
  });

  return {
    policy,
    level,
    setLevel,
    rules,
    updateRules,
    loading,
    saving,
    dirty,
    lastUpdatedAt,
    fetchPolicy,
    savePolicy,
    applyPolicy,
    startListening,
    stopListening,
  };
}
