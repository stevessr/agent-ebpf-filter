import { onUnmounted, ref, watch } from "vue";
import type { ProcessInfo } from "../../../composables/monitor/useProcessObserver";

type ResourceField = "cpu" | "mem";
export type ResourceSeries = { name: string; data: [number, number][] };
export type ResourceHistory = { time: number; series: ResourceSeries[] };

const SAMPLE_INTERVAL_MS = 3_000;
const MAX_HISTORY_SAMPLES = 60;

export function updateResourceHistory(
  history: ResourceHistory,
  processes: ProcessInfo[],
  field: ResourceField,
  now: number,
): ResourceHistory {
  const cutoff = now - MAX_HISTORY_SAMPLES * SAMPLE_INTERVAL_MS;
  const activeNames = new Set(
    processes.map((process) => `[${process.pid}] ${process.name}`),
  );
  const seriesByName = new Map(
    history.series
      .filter((series) => activeNames.has(series.name))
      .map((series) => ({
        name: series.name,
        data: series.data.filter(([time]) => time >= cutoff),
      }))
      .filter((series) => series.data.length > 0)
      .map((series) => [series.name, series] as const),
  );

  for (const process of processes) {
    const name = `[${process.pid}] ${process.name}`;
    const series = seriesByName.get(name) ?? { name, data: [] };
    series.data.push([now, process[field] ?? 0]);
    if (series.data.length > MAX_HISTORY_SAMPLES) series.data.shift();
    seriesByName.set(name, series);
  }
  return { time: now, series: [...seriesByName.values()] };
}

export function useProcessResourceHistory(options: {
  processes: () => ProcessInfo[];
  treePids: () => Set<number>;
}) {
  const cpuHistory = ref<ResourceHistory>({ time: 0, series: [] });
  const memHistory = ref<ResourceHistory>({ time: 0, series: [] });
  let sampleTimer: ReturnType<typeof setInterval> | null = null;

  const currentPidKey = () =>
    [...options.treePids()].sort((a, b) => a - b).join(",");

  const sample = () => {
    const pids = options.treePids();
    const processes = options.processes().filter((process) =>
      pids.has(process.pid),
    );
    const now = Date.now();
    cpuHistory.value = updateResourceHistory(
      cpuHistory.value,
      processes,
      "cpu",
      now,
    );
    memHistory.value = updateResourceHistory(
      memHistory.value,
      processes,
      "mem",
      now,
    );
  };

  watch(
    currentPidKey,
    (pidKey) => {
      if (sampleTimer) clearInterval(sampleTimer);
      sampleTimer = null;
      if (!pidKey) {
        cpuHistory.value = { time: 0, series: [] };
        memHistory.value = { time: 0, series: [] };
        return;
      }
      sample();
      sampleTimer = setInterval(sample, SAMPLE_INTERVAL_MS);
    },
    { immediate: true },
  );

  onUnmounted(() => {
    if (sampleTimer) clearInterval(sampleTimer);
  });

  return { cpuHistory, memHistory };
}
