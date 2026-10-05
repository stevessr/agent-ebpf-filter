import { describe, expect, test } from "bun:test";
import config from "../vite.config";

describe("redaction policy development proxy", () => {
  test("routes the API to the backend while preserving HTML navigation", () => {
    const proxies = config.server!.proxy!;
    const key = Object.keys(proxies).find(
      (pattern) => pattern.startsWith("^") && new RegExp(pattern).test("/config/redaction-policy"),
    );
    expect(key).toBeDefined();
    const proxy = proxies[key!];
    if (typeof proxy === "string") throw new Error("Expected proxy options");
    expect(proxy.target).toMatch(/^http:\/\/localhost:\d+$/);
    const bypass = proxy.bypass as (req: { headers: { accept: string }; method: string }) => unknown;
    for (const method of ["GET", "PUT"]) {
      expect(bypass({ method, headers: { accept: "application/json" } })).toBeUndefined();
    }
    expect(bypass({ method: "GET", headers: { accept: "text/html" } })).toBe("/index.html");
  });
});
