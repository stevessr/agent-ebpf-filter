import { describe, expect, test } from "bun:test";

import {
  RENEW_MONITORING_MODULES,
  disabledEventTypesForModules,
  enabledEventTypesForModules,
  estimateMonitoringWeight,
  monitoringWeightLabel,
  profileByKey,
} from "../src/composables/renew/monitoringPresets";

describe("Renew monitoring presets", () => {
  test("daily profile excludes high-frequency monitoring groups", () => {
    const daily = profileByKey("daily");
    expect(daily.modules).toContain("process");
    expect(daily.modules).toContain("file-changes");
    expect(daily.modules).toContain("network");
    expect(daily.modules).not.toContain("file-access");
    expect(daily.modules).not.toContain("network-detail");
    expect(daily.modules).not.toContain("deep-syscall");

    const enabled = enabledEventTypesForModules(daily.modules);
    const fileAccess = RENEW_MONITORING_MODULES.find(
      (module) => module.key === "file-access",
    )!;
    expect(fileAccess.eventTypes.every((type) => !enabled.has(type))).toBe(true);
  });

  test("disabled list is the complement of enabled module event types", () => {
    const processOnly = ["process"];
    const enabled = enabledEventTypesForModules(processOnly);
    const disabled = new Set(disabledEventTypesForModules(processOnly));

    for (const module of RENEW_MONITORING_MODULES) {
      for (const type of module.eventTypes) {
        if (enabled.has(type)) expect(disabled.has(type)).toBe(false);
        else expect(disabled.has(type)).toBe(true);
      }
    }
  });

  test("deep monitoring estimates more overhead than daily monitoring", () => {
    const daily = profileByKey("daily");
    const deep = profileByKey("deep");

    const dailyWeight = estimateMonitoringWeight(daily.modules, {
      statsIntervalMs: daily.statsIntervalMs,
      loopDetection: daily.loopDetection,
      signalProcessing: daily.signalProcessing,
      researchProcessing: daily.researchProcessing,
      tlsCapture: false,
      persistence: true,
    });
    const deepWeight = estimateMonitoringWeight(deep.modules, {
      statsIntervalMs: deep.statsIntervalMs,
      loopDetection: deep.loopDetection,
      signalProcessing: deep.signalProcessing,
      researchProcessing: deep.researchProcessing,
      tlsCapture: true,
      persistence: true,
    });

    expect(deepWeight).toBeGreaterThan(dailyWeight);
    expect(monitoringWeightLabel(deepWeight).tone).toBe("high");
  });
});
