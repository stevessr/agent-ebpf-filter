import { describe, expect, test } from "bun:test";

import {
  RENEW_MONITORING_MODULES,
  disabledEventTypesForModules,
  enabledEventTypesForModules,
  estimateMonitoringWeight,
  monitoringWeightLabel,
  profileByKey,
} from "../src/composables/renew/monitoringPresets";

describe("Renew monitoring profiles", () => {
  test("daily profile keeps high-frequency detail modules off", () => {
    const daily = profileByKey("daily");
    expect(daily.modules).toContain("process");
    expect(daily.modules).toContain("file-changes");
    expect(daily.modules).toContain("network");
    expect(daily.modules).not.toContain("file-access");
    expect(daily.modules).not.toContain("network-detail");
    expect(daily.modules).not.toContain("deep-syscall");
    expect(daily.researchProcessing).toBe(false);
    expect(daily.statsIntervalMs).toBe(5000);
  });

  test("module enable and disable sets are complementary", () => {
    const lite = profileByKey("lite");
    const enabled = enabledEventTypesForModules(lite.modules);
    const disabled = new Set(disabledEventTypesForModules(lite.modules));
    const all = new Set(
      RENEW_MONITORING_MODULES.flatMap((module) => module.eventTypes),
    );

    for (const eventType of all) {
      expect(enabled.has(eventType) === disabled.has(eventType)).toBe(false);
    }
  });

  test("deep collection has a visibly higher overhead score", () => {
    const lite = profileByKey("lite");
    const deep = profileByKey("deep");

    const liteWeight = estimateMonitoringWeight(lite.modules, {
      statsIntervalMs: lite.statsIntervalMs,
      loopDetection: lite.loopDetection,
      signalProcessing: lite.signalProcessing,
      researchProcessing: lite.researchProcessing,
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

    expect(deepWeight).toBeGreaterThan(liteWeight);
    expect(monitoringWeightLabel(liteWeight).tone).toBe("low");
    expect(monitoringWeightLabel(deepWeight).tone).toBe("high");
  });
});
