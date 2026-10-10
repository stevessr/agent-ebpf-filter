import { describe, expect, test } from "bun:test";
import config, { resolveBackendUrl } from "../vite.config";

describe("redaction policy development proxy", () => {
  test("routes the API to the backend while preserving HTML navigation", () => {
    const proxies = config.server!.proxy!;
    const key = Object.keys(proxies).find(
      (pattern) => pattern.startsWith("^") && new RegExp(pattern).test("/config/redaction-policy"),
    );
    expect(key).toBeDefined();
    const proxy = proxies[key!];
    if (typeof proxy === "string") throw new Error("Expected proxy options");
    if (process.env.VITE_AGENT_BACKEND_URL?.trim()) {
      expect(proxy.target).toBe(
        resolveBackendUrl(process.env.VITE_AGENT_BACKEND_URL),
      );
    } else {
      const target = new URL(proxy.target!);
      expect(target.protocol).toBe("http:");
      expect(target.hostname).toBe("127.0.0.1");
      expect(Number(target.port)).toBeGreaterThan(0);
    }
    const bypass = proxy.bypass as (req: { headers: { accept: string }; method: string }) => unknown;
    for (const method of ["GET", "PUT"]) {
      expect(bypass({ method, headers: { accept: "application/json" } })).toBeUndefined();
    }
    expect(bypass({ method: "GET", headers: { accept: "text/html" } })).toBe("/index.html");
  });
});

describe("backend URL resolution", () => {
  test("defaults to a loopback backend and honors the port file value", () => {
    expect(resolveBackendUrl()).toBe("http://127.0.0.1:8080");
    expect(resolveBackendUrl(undefined, 8091)).toBe("http://127.0.0.1:8091");
  });
  test("honors an explicit backend URL without trailing slashes", () => {
    expect(resolveBackendUrl(" https://backend.example:9443/// ", 8091)).toBe(
      "https://backend.example:9443",
    );
  });
  test("invalid or missing port file values fall back safely", () => {
    for (const port of [NaN, 0, -1, 65536, 1.5]) {
      expect(resolveBackendUrl(" ", port)).toBe("http://127.0.0.1:8080");
    }
  });
});
