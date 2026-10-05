import { computed, ref } from "vue";
import axios from "axios";
import type { CollectorHealthResponse, RuntimeConfigResponse, RuntimeSettings } from "../../types/config";
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
  return normalizeStatsInterval(window.localStorage.getItem(STATS_INTERVAL_KEY));
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
  const applying = ref(false);
  const error = ref("");

  const enabledModuleKeys = computed(() =>
    RENEW_MONITORING_MODULES.filter((module) =>
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

  const overhead = computed(() => monitoringWeightLabel(monitoringWeight.value));

  const activeProfileKey = computed<RenewMonitoringProfileKey | "custom">(() => {
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
      if (
        runtimeEnabled("researchProcessing") !== profile.researchProcessing
      )
        continue;
      return profile.key;
    }
    return "custom";
  });

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
  };

  const fetchState = async () => {
    loading.value = true;
    error.value = "";
    try {
      const runtimeResponse = await axios.get<RuntimeConfigResponse>(
        "/config/runtime",
      );
      applyRuntimeResponse(runtimeResponse.data);
      void fetchCollectorHealth();
    } catch (cause: any) {
      error.value =
        cause?.response?.data?.error ||
        cause?.message ||
        "无法加载监控配置";
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
    if (!module) return;

    applying.value = true;
    error.value = "";
    try {
      const disabled = new Set(disabledEventTypes.value);
      for (const eventType of module.eventTypes) {
        if (enabled) disabled.delete(eventType);
        else disabled.add(eventType);
      }
      await putDisabledEventTypes(disabled);
    } catch (cause: any) {
      error.value =
        cause?.response?.data?.error ||
        cause?.message ||
        "更新监控模块失败";
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
    if (!runtimeSettings.value) return;
    applying.value = true;
    error.value = "";
    try {
      const response = await axios.put<RuntimeConfigResponse>(
        "/config/runtime",
        runtimePatch(key, enabled),
      );
      applyRuntimeResponse(response.data);
    } catch (cause: any) {
      error.value =
        cause?.response?.data?.error ||
        cause?.message ||
        "更新运行时监控配置失败";
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

      const previousDisabled = new Set(disabledEventTypes.value);
      await putDisabledEventTypes(disabled);
      try {
        if (runtimePayload) {
          const response = await axios.put<RuntimeConfigResponse>(
            "/config/runtime",
            runtimePayload,
          );
          applyRuntimeResponse(response.data);
        }
      } catch (cause) {
        // Keep a profile switch atomic from the user's perspective. If the
        // runtime gate update fails, restore the event filter configuration
        // instead of leaving a half-applied monitoring profile.
        try {
          await putDisabledEventTypes(previousDisabled);
        } catch (_) {
          // Preserve the original failure below; fetchState can reconcile the
          // UI with the backend if even the rollback request fails.
          void fetchState();
        }
        throw cause;
      }
      setStatsInterval(profile.statsIntervalMs);
    } catch (cause: any) {
      error.value =
        cause?.response?.data?.error ||
        cause?.message ||
        "应用监控档位失败";
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
