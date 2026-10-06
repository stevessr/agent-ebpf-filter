import { computed, ref } from "vue";
import axios from "axios";
import { backendError, renewToast } from "./notify";
import type {
  CollectorHealthResponse,
  RuntimeConfigResponse,
  RuntimeSettings,
} from "../../types/config";
import {
  RENEW_KERNEL_MONITOR_EVENT_TYPES,
  RENEW_MONITORING_MODULES,
  RENEW_MONITORING_PROFILES,
  disabledEventTypesForModules,
  estimateMonitoringWeight,
  monitoringWeightLabel,
  profileByKey,
  type RenewMonitoringProfileKey,
} from "./monitoringPresets";

const STATS_INTERVAL_KEY = "agent-ebpf.renew.statsIntervalMs";

const normalizeStatsInterval = (value: unknown) => {
  const n = Number(value);
  return [2_000, 5_000, 10_000, 30_000].includes(n) ? n : 5_000;
};

const initialStatsInterval = () => {
  if (typeof window === "undefined") return 5_000;
  return normalizeStatsInterval(
    window.localStorage.getItem(STATS_INTERVAL_KEY),
  );
};

export type RenewRuntimeToggleKey =
  | "loopDetection"
  | "signalProcessing"
  | "researchProcessing"
  | "tlsCapture"
  | "persistence";

export function useRenewMonitoringControls() {
  const disabledEventTypes = ref<Set<number>>(new Set());
  const runtimeSettings = ref<RuntimeSettings | null>(null);
  const statsIntervalMs = ref(initialStatsInterval());
  const collectorHealth = ref<Partial<CollectorHealthResponse> | null>(null);
  const persistedEventLogPath = ref("");
  const persistedEventLogAlive = ref(false);
  const loading = ref(false);
  const ready = ref(false);
  const applying = ref(false);
  const error = ref("");

  const enabledModuleKeys = computed(() =>
    RENEW_MONITORING_MODULES.filter(
      (module) =>
        ready.value &&
        module.eventTypes.every((type) => !disabledEventTypes.value.has(type)),
    ).map((module) => module.key),
  );

  const runtimeEnabled = (key: RenewRuntimeToggleKey) => {
    const settings = runtimeSettings.value;
    if (!settings) return false;
    switch (key) {
      case "loopDetection":
        return Boolean(settings.loopDetection?.enabled);
      case "signalProcessing":
        return Boolean(settings.signalProcessing?.enabled);
      case "researchProcessing":
        return Boolean(settings.researchProcessing?.enabled);
      case "tlsCapture":
        return Boolean(settings.tlsCaptureEnabled);
      case "persistence":
        return Boolean(settings.logPersistenceEnabled);
    }
  };

  const monitoringWeight = computed(() =>
    estimateMonitoringWeight(enabledModuleKeys.value, {
      statsIntervalMs: statsIntervalMs.value,
      loopDetection: runtimeEnabled("loopDetection"),
      signalProcessing: runtimeEnabled("signalProcessing"),
      researchProcessing: runtimeEnabled("researchProcessing"),
      tlsCapture: runtimeEnabled("tlsCapture"),
      persistence: runtimeEnabled("persistence"),
    }),
  );

  const overhead = computed(() =>
    monitoringWeightLabel(monitoringWeight.value),
  );

  const activeProfileKey = computed<RenewMonitoringProfileKey | "custom">(
    () => {
      const enabled = new Set(enabledModuleKeys.value);
      for (const profile of RENEW_MONITORING_PROFILES) {
        if (
          enabled.size !== profile.modules.length ||
          profile.modules.some((key) => !enabled.has(key))
        ) {
          continue;
        }
        if (statsIntervalMs.value !== profile.statsIntervalMs) continue;
        if (runtimeEnabled("loopDetection") !== profile.loopDetection) continue;
        if (runtimeEnabled("signalProcessing") !== profile.signalProcessing)
          continue;
        if (runtimeEnabled("researchProcessing") !== profile.researchProcessing)
          continue;
        return profile.key;
      }
      return "custom";
    },
  );

  const fetchCollectorHealth = async () => {
    try {
      const response = await axios.get<CollectorHealthResponse>(
        "/system/collector-health",
      );
      collectorHealth.value = response.data;
    } catch (_) {
      collectorHealth.value = null;
    }
  };

  const applyRuntimeResponse = (payload: RuntimeConfigResponse) => {
    runtimeSettings.value = payload.runtime;
    disabledEventTypes.value = new Set(
      payload.runtime.disabledEventTypes || [],
    );
    persistedEventLogPath.value = payload.persistedEventLogPath || "";
    persistedEventLogAlive.value = Boolean(payload.persistedEventLogAlive);
    ready.value = Boolean(payload.runtime);
  };

  const fetchState = async () => {
    if (applying.value || loading.value) return;
    loading.value = true;
    error.value = "";
    try {
      const runtimeResponse =
        await axios.get<RuntimeConfigResponse>("/config/runtime");
      applyRuntimeResponse(runtimeResponse.data);
      void fetchCollectorHealth();
    } catch (cause) {
      ready.value = false;
      error.value = backendError(cause, "无法加载监控配置");
    } finally {
      loading.value = false;
    }
  };

  const putDisabledEventTypes = async (disabled: Set<number>) => {
    const next = [...disabled].sort((a, b) => a - b);
    const response = await axios.put<RuntimeConfigResponse>("/config/runtime", {
      disabledEventTypes: next,
    });
    applyRuntimeResponse(response.data);
    disabledEventTypes.value = new Set(
      response.data.runtime.disabledEventTypes || next,
    );
  };

  const setModuleEnabled = async (moduleKey: string, enabled: boolean) => {
    const module = RENEW_MONITORING_MODULES.find(
      (candidate) => candidate.key === moduleKey,
    );
    if (!module || !ready.value || applying.value || loading.value) return;

    applying.value = true;
    error.value = "";
    try {
      const disabled = new Set(disabledEventTypes.value);
      for (const eventType of module.eventTypes) {
        if (enabled) disabled.delete(eventType);
        else disabled.add(eventType);
      }
      await putDisabledEventTypes(disabled);
      renewToast.success(
        enabled
          ? `已启用监控模块：${moduleKey}`
          : `已停用监控模块：${moduleKey}`,
      );
    } catch (cause) {
      error.value = backendError(cause, "更新监控模块失败");
      throw cause;
    } finally {
      applying.value = false;
    }
  };

  const runtimePatch = (
    key: RenewRuntimeToggleKey,
    enabled: boolean,
  ): Record<string, unknown> => {
    const settings = runtimeSettings.value;
    if (!settings) return {};
    switch (key) {
      case "loopDetection":
        return {
          loopDetection: { ...settings.loopDetection, enabled },
        };
      case "signalProcessing":
        return {
          signalProcessing: { ...settings.signalProcessing, enabled },
        };
      case "researchProcessing":
        return {
          researchProcessing: { ...settings.researchProcessing, enabled },
        };
      case "tlsCapture":
        return { tlsCaptureEnabled: enabled };
      case "persistence":
        return { logPersistenceEnabled: enabled };
    }
  };

  const setRuntimeEnabled = async (
    key: RenewRuntimeToggleKey,
    enabled: boolean,
  ) => {
    if (
      !ready.value ||
      !runtimeSettings.value ||
      applying.value ||
      loading.value
    )
      return;
    applying.value = true;
    error.value = "";
    try {
      const response = await axios.put<RuntimeConfigResponse>(
        "/config/runtime",
        runtimePatch(key, enabled),
      );
      applyRuntimeResponse(response.data);
      renewToast.success(
        enabled ? `已启用运行时开关：${key}` : `已停用运行时开关：${key}`,
      );
    } catch (cause) {
      error.value = backendError(cause, "更新运行时监控配置失败");
      throw cause;
    } finally {
      applying.value = false;
    }
  };

  const setStatsInterval = (intervalMs: number) => {
    statsIntervalMs.value = normalizeStatsInterval(intervalMs);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(
        STATS_INTERVAL_KEY,
        String(statsIntervalMs.value),
      );
    }
  };

  const applyProfile = async (key: RenewMonitoringProfileKey) => {
    if (!ready.value || applying.value || loading.value) return;
    const profile = profileByKey(key);
    applying.value = true;
    error.value = "";
    try {
      // Preserve disabled user-space event types outside the kernel monitoring
      // groups that Renew owns.
      const disabled = new Set(
        [...disabledEventTypes.value].filter(
          (type) => !RENEW_KERNEL_MONITOR_EVENT_TYPES.includes(type),
        ),
      );
      for (const type of disabledEventTypesForModules(profile.modules)) {
        disabled.add(type);
      }

      const settings = runtimeSettings.value;
      const runtimePayload = settings
        ? {
            loopDetection: {
              ...settings.loopDetection,
              enabled: profile.loopDetection,
            },
            signalProcessing: {
              ...settings.signalProcessing,
              enabled: profile.signalProcessing,
            },
            researchProcessing: {
              ...settings.researchProcessing,
              enabled: profile.researchProcessing,
            },
          }
        : null;

      const response = await axios.put<RuntimeConfigResponse>(
        "/config/runtime",
        {
          ...(runtimePayload || {}),
          disabledEventTypes: [...disabled].sort((a, b) => a - b),
        },
      );
      applyRuntimeResponse(response.data);
      setStatsInterval(profile.statsIntervalMs);
      renewToast.success(`已应用监控档位：${key}`);
    } catch (cause) {
      error.value = backendError(cause, "应用监控档位失败");
      throw cause;
    } finally {
      applying.value = false;
    }
  };

  return {
    modules: RENEW_MONITORING_MODULES,
    profiles: RENEW_MONITORING_PROFILES,
    disabledEventTypes,
    runtimeSettings,
    statsIntervalMs,
    collectorHealth,
    persistedEventLogPath,
    persistedEventLogAlive,
    loading,
    ready,
    applying,
    error,
    enabledModuleKeys,
    activeProfileKey,
    monitoringWeight,
    overhead,
    fetchState,
    fetchCollectorHealth,
    runtimeEnabled,
    setModuleEnabled,
    setRuntimeEnabled,
    setStatsInterval,
    applyProfile,
  };
}
