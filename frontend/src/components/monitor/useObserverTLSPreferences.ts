import { ref, watch } from "vue";

const SSL_SKIP_KEY = "observe-skip-ssl";
const SSL_AUTO_ATTACH_KEY = "observe-auto-attach";

const readStoredBool = (key: string, fallback: boolean): boolean => {
  try {
    const value = localStorage.getItem(key);
    return value === null ? fallback : value === "true";
  } catch {
    return fallback;
  }
};

export function useObserverTLSPreferences() {
  const skipSSL = ref(readStoredBool(SSL_SKIP_KEY, false));
  const autoAttach = ref(readStoredBool(SSL_AUTO_ATTACH_KEY, false));

  watch(skipSSL, (value) => {
    try {
      localStorage.setItem(SSL_SKIP_KEY, String(value));
    } catch {
      // Ignore unavailable browser storage.
    }
  });
  watch(autoAttach, (value) => {
    try {
      localStorage.setItem(SSL_AUTO_ATTACH_KEY, String(value));
    } catch {
      // Ignore unavailable browser storage.
    }
  });

  return { skipSSL, autoAttach };
}
