export const autoTunePublicApi = <
  T extends {
    applyAutoTuneStatus: (...args: any[]) => unknown;
    stopAutoTunePolling: (...args: any[]) => unknown;
  },
>(api: T): Omit<T, "applyAutoTuneStatus" | "stopAutoTunePolling"> => {
  const { applyAutoTuneStatus, stopAutoTunePolling, ...publicApi } = api;
  void applyAutoTuneStatus;
  void stopAutoTunePolling;
  return publicApi;
};

export const mlSampleActionsPublicApi = <T extends { dispose: () => void }>(
  actions: T,
): Omit<T, "dispose"> => {
  const { dispose, ...publicApi } = actions;
  void dispose;
  return publicApi;
};
