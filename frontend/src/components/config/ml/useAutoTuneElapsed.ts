import { ref, watch, onBeforeUnmount } from "vue";
import type { Ref } from "vue";

/**
 * Elapsed-time tracker for the auto-tune progress indicator.
 * Extracted from ConfigMLParamsTab.vue.
 */
export function useAutoTuneElapsed(autoTuneInProgress: Ref<boolean>) {
  const autoTuneStartTime = ref(0);
  const autoTuneElapsed = ref("");
  let autoTuneElapsedTimer: ReturnType<typeof setInterval> | null = null;

  watch(autoTuneInProgress, (running) => {
    if (running) {
      autoTuneStartTime.value = Date.now();
      autoTuneElapsed.value = "0s";
      autoTuneElapsedTimer = setInterval(() => {
        const sec = Math.floor((Date.now() - autoTuneStartTime.value) / 1000);
        autoTuneElapsed.value =
          sec < 60 ? `${sec}s` : `${Math.floor(sec / 60)}m${sec % 60}s`;
      }, 1000);
    } else {
      if (autoTuneElapsedTimer) {
        clearInterval(autoTuneElapsedTimer);
        autoTuneElapsedTimer = null;
      }
    }
  });

  onBeforeUnmount(() => {
    if (autoTuneElapsedTimer) {
      clearInterval(autoTuneElapsedTimer);
      autoTuneElapsedTimer = null;
    }
  });

  return { autoTuneElapsed };
}
