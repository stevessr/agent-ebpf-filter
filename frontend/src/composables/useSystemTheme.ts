import { readonly, ref } from "vue";

// One reactive source of truth for Ant Design, custom surfaces and editors.
// No saved override: changes from the OS are applied immediately.
const mediaQuery =
  typeof window !== "undefined" && typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-color-scheme: dark)")
    : null;

const dark = ref(mediaQuery?.matches ?? false);
export const systemIsDark = readonly(dark);

const applySystemTheme = () => {
  dark.value = mediaQuery?.matches ?? false;
  if (typeof document !== "undefined") {
    document.documentElement.dataset.theme = dark.value ? "dark" : "light";
    document.documentElement.style.colorScheme = dark.value ? "dark" : "light";
  }
};

// Set the initial appearance before Vue mounts to avoid a light-theme flash.
applySystemTheme();

/** Call once from the root app; cleanup prevents listeners on remount/HMR. */
export const watchSystemTheme = () => {
  applySystemTheme();
  mediaQuery?.addEventListener("change", applySystemTheme);
  return () => mediaQuery?.removeEventListener("change", applySystemTheme);
};
