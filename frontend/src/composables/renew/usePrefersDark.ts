import { onMounted, onUnmounted, readonly, ref } from "vue";

/**
 * Tracks the user agent's `prefers-color-scheme: dark` setting reactively.
 *
 * Renew ships its own light palette as CSS custom properties, but Ant Design
 * components (a-drawer, a-switch, a-segmented, a-input) derive their colors
 * from the ConfigProvider theme, so they need the same signal in JS.
 */
export function usePrefersDark() {
  const prefersDark = ref(false);

  if (
    typeof window === "undefined" ||
    typeof window.matchMedia !== "function"
  ) {
    return readonly(prefersDark);
  }

  const media = window.matchMedia("(prefers-color-scheme: dark)");
  prefersDark.value = media.matches;

  const sync = (event: MediaQueryListEvent) => {
    prefersDark.value = event.matches;
  };

  onMounted(() => media.addEventListener("change", sync));
  onUnmounted(() => media.removeEventListener("change", sync));

  return readonly(prefersDark);
}
