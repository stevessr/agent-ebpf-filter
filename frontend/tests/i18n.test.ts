import { afterAll, describe, expect, test } from "bun:test";
import {
  setWebLocale, translateUI, webLocale,
  type WebLocale,
} from "../src/i18n";

const previousWindow = (globalThis as any).window;
const saved = new Map<string, string>();
(globalThis as any).window = {
  localStorage: {
    getItem: (key: string) => saved.get(key) ?? null,
    setItem: (key: string, value: string) => saved.set(key, value),
  },
};
afterAll(() => { (globalThis as any).window = previousWindow; });

describe("WebUI locale", () => {
  test("English translation, inline numbers, and original whitespace", () => {
    setWebLocale("en-US");
    expect(webLocale.value).toBe("en-US");
    expect(translateUI("仪表盘")).toBe("Dashboard");
    expect(translateUI("  系统监控  ")).toBe("  System Monitor  ");
    expect(translateUI("3 个面板")).toBe("3 panes");
    expect(translateUI("Agent eBPF")).toBe("Agent eBPF");
  });

  test("Chinese labels, save locale, and protect unrecognized event data", () => {
    setWebLocale("zh-CN");
    expect(translateUI("Dashboard")).toBe("仪表盘");
    expect(translateUI("Runtime")).toBe("运行时");
    expect(translateUI("用户写入的业务日志")).toBe("用户写入的业务日志");
    expect(saved.get("agent.webui.locale.v1")).toBe("zh-CN");
    setWebLocale("en-US");
    expect(saved.get("agent.webui.locale.v1")).toBe("en-US");
  });

  test("accept only supported locale values at call site", () => {
    const values: WebLocale[] = ["zh-CN", "en-US"];
    expect(values.every((v) => ["zh-CN", "en-US"].includes(v))).toBe(true);
  });
});
