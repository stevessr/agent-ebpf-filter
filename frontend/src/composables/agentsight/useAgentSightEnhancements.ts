import { ref, watch, type Ref } from "vue";

/**
 * Composable for managing AgentSight performance metrics
 */
export function useAgentSightPerformance() {
  const processingTime = ref(0);
  const lastUpdateTime = ref(Date.now());
  const updateCount = ref(0);
  const memoryBaseline = ref(0);

  // Track memory usage (if available)
  const estimateMemoryUsage = () => {
    if ('memory' in performance && 'usedJSHeapSize' in (performance as any).memory) {
      return (performance as any).memory.usedJSHeapSize;
    }
    return 0;
  };

  const startProcessing = () => {
    lastUpdateTime.value = Date.now();
    if (!memoryBaseline.value) {
      memoryBaseline.value = estimateMemoryUsage();
    }
  };

  const endProcessing = () => {
    processingTime.value = Date.now() - lastUpdateTime.value;
    updateCount.value++;
  };

  const getMemoryDelta = () => {
    const current = estimateMemoryUsage();
    return current - memoryBaseline.value;
  };

  const reset = () => {
    processingTime.value = 0;
    updateCount.value = 0;
    memoryBaseline.value = estimateMemoryUsage();
  };

  return {
    processingTime,
    updateCount,
    startProcessing,
    endProcessing,
    getMemoryDelta,
    reset,
  };
}

/**
 * Composable for handling errors in AgentSight
 */
export function useAgentSightError() {
  const error = ref<string | null>(null);
  const errorDetails = ref<any>(null);

  const setError = (message: string, details?: any) => {
    error.value = message;
    errorDetails.value = details;
    console.error('[AgentSight Error]', message, details);
  };

  const clearError = () => {
    error.value = null;
    errorDetails.value = null;
  };

  const handleError = (err: unknown, context: string) => {
    const message = err instanceof Error ? err.message : String(err);
    setError(`${context}: ${message}`, err);
  };

  return {
    error,
    errorDetails,
    setError,
    clearError,
    handleError,
  };
}

/**
 * Composable for progressive loading of large datasets
 */
export function useProgressiveLoad<T>(
  items: Ref<T[]>,
  batchSize = 100,
  delay = 50
) {
  const visibleItems = ref<T[]>([]) as Ref<T[]>;
  const isLoading = ref(false);
  const loadedCount = ref(0);

  const loadBatch = async () => {
    if (loadedCount.value >= items.value.length) {
      isLoading.value = false;
      return;
    }

    const nextBatch = items.value.slice(
      loadedCount.value,
      loadedCount.value + batchSize
    );

    visibleItems.value = [...visibleItems.value, ...nextBatch];
    loadedCount.value += nextBatch.length;

    if (loadedCount.value < items.value.length) {
      await new Promise((resolve) => setTimeout(resolve, delay));
      await loadBatch();
    } else {
      isLoading.value = false;
    }
  };

  const startLoading = () => {
    visibleItems.value = [];
    loadedCount.value = 0;
    isLoading.value = true;
    loadBatch();
  };

  // Watch for changes in source items
  watch(
    () => items.value.length,
    () => {
      if (items.value.length > batchSize) {
        startLoading();
      } else {
        visibleItems.value = items.value;
        isLoading.value = false;
      }
    },
    { immediate: true }
  );

  return {
    visibleItems,
    isLoading,
    loadedCount,
    startLoading,
  };
}

/**
 * Composable for keyboard shortcuts
 */
export function useAgentSightKeyboard(handlers: {
  onRefresh?: () => void;
  onClear?: () => void;
  onSearch?: () => void;
  onExpandAll?: () => void;
  onCollapseAll?: () => void;
}) {
  const handleKeydown = (event: KeyboardEvent) => {
    // Ctrl/Cmd + R: Refresh
    if ((event.ctrlKey || event.metaKey) && event.key === 'r') {
      event.preventDefault();
      handlers.onRefresh?.();
    }
    // Ctrl/Cmd + K: Clear
    else if ((event.ctrlKey || event.metaKey) && event.key === 'k') {
      event.preventDefault();
      handlers.onClear?.();
    }
    // Ctrl/Cmd + F: Search
    else if ((event.ctrlKey || event.metaKey) && event.key === 'f') {
      event.preventDefault();
      handlers.onSearch?.();
    }
    // Ctrl/Cmd + E: Expand all
    else if ((event.ctrlKey || event.metaKey) && event.key === 'e') {
      event.preventDefault();
      handlers.onExpandAll?.();
    }
    // Ctrl/Cmd + Shift + E: Collapse all
    else if ((event.ctrlKey || event.metaKey) && event.shiftKey && event.key === 'E') {
      event.preventDefault();
      handlers.onCollapseAll?.();
    }
  };

  const enable = () => {
    window.addEventListener('keydown', handleKeydown);
  };

  const disable = () => {
    window.removeEventListener('keydown', handleKeydown);
  };

  return {
    enable,
    disable,
  };
}
