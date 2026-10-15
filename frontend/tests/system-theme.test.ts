import { expect, test } from "bun:test";

test("WebUI follows OS light/dark changes without reload or stale listeners", async () => {
  const previousWindow = (globalThis as any).window;
  const previousDocument = (globalThis as any).document;
  let listener: (() => void) | undefined;
  const media = {
    matches: false,
    addEventListener: (_event: string, callback: () => void) => {
      listener = callback;
    },
    removeEventListener: (_event: string, callback: () => void) => {
      if (listener === callback) listener = undefined;
    },
  };
  const root = {
    dataset: {} as Record<string, string>,
    style: { colorScheme: "" },
  };
  try {
    (globalThis as any).window = { matchMedia: () => media };
    // Vue runtime-dom detects document while importing; create the DOM stub
    // only after Vue has loaded, so no createElement implementation is needed.
    (globalThis as any).document = undefined;
    const { systemIsDark, watchSystemTheme } = await import(
      "../src/composables/useSystemTheme"
    );
    (globalThis as any).document = { documentElement: root };
    const stop = watchSystemTheme();
    expect(root.dataset.theme).toBe("light");
    expect(systemIsDark.value).toBe(false);
    media.matches = true;
    listener?.();
    expect(systemIsDark.value).toBe(true);
    expect(root.dataset.theme).toBe("dark");
    expect(root.style.colorScheme).toBe("dark");

    media.matches = false;
    listener?.();
    expect(root.dataset.theme).toBe("light");
    expect(root.style.colorScheme).toBe("light");

    stop();
    expect(listener).toBeUndefined();
  } finally {
    (globalThis as any).window = previousWindow;
    (globalThis as any).document = previousDocument;
  }
});
