import type { LocationQuery } from 'vue-router';
import type { ExecutionGraphFilterState } from '../../types/executionGraph';

export const timePresetOptions: ExecutionGraphFilterState['timePreset'][] = ['all', '15m', '1h', '6h', '24h', '7d', 'custom'];

export const defaultFilters = (): ExecutionGraphFilterState => ({
  limit: 600,
  agentRunId: '',
  toolCallId: '',
  traceId: '',
  pid: '',
  processTree: true,
  comm: '',
  toolName: '',
  path: '',
  domain: '',
  decision: '',
  riskMin: 0,
  timePreset: '24h',
  since: '',
  until: '',
});

const singleQuery = (value: unknown) => Array.isArray(value) ? (value[0] ?? '') : (value ?? '');

export const filtersFromRoute = (query: LocationQuery): ExecutionGraphFilterState => {
  const defaults = defaultFilters();
  const parsedLimit = Number(singleQuery(query.limit));
  const parsedRisk = Number(singleQuery(query.risk_min));
  const timePreset = String(singleQuery(query.timePreset || query.time_preset || defaults.timePreset)).trim() as ExecutionGraphFilterState['timePreset'];
  const processTreeRaw = String(singleQuery(query.process_tree)).trim().toLowerCase();
  return {
    ...defaults,
    limit: Number.isFinite(parsedLimit) && parsedLimit > 0 ? parsedLimit : defaults.limit,
    agentRunId: String(singleQuery(query.agent_run_id)).trim(),
    toolCallId: String(singleQuery(query.tool_call_id)).trim(),
    traceId: String(singleQuery(query.trace_id)).trim(),
    pid: String(singleQuery(query.pid)).trim(),
    processTree: processTreeRaw === '' ? defaults.processTree : ['1', 'true', 'yes', 'on'].includes(processTreeRaw),
    comm: String(singleQuery(query.comm)).trim(),
    toolName: String(singleQuery(query.tool_name)).trim(),
    path: String(singleQuery(query.path)).trim(),
    domain: String(singleQuery(query.domain)).trim(),
    decision: String(singleQuery(query.decision)).trim(),
    riskMin: Number.isFinite(parsedRisk) && parsedRisk > 0 ? parsedRisk : defaults.riskMin,
    timePreset: timePresetOptions.includes(timePreset) ? timePreset : defaults.timePreset,
    since: String(singleQuery(query.since)).trim(),
    until: String(singleQuery(query.until)).trim(),
  };
};

export interface UseGraphFiltersOptions {
  route: { query: LocationQuery };
  router: { push: (...args: any[]) => Promise<unknown> };
  filters: ExecutionGraphFilterState;
  selectedProcessPid: { value: number | null };
  replayPath: { value: string };
  buildPresetSince: (preset: ExecutionGraphFilterState['timePreset']) => string;
  syncRouteQuery: () => Promise<void>;
  connectGraphSocket: () => void;
}

export function useGraphFilters(opts: UseGraphFiltersOptions) {
  const { filters, selectedProcessPid, replayPath, syncRouteQuery, connectGraphSocket } = opts;

  const applyFilters = async () => {
    await syncRouteQuery();
    connectGraphSocket();
  };

  const resetFilters = async () => {
    Object.assign(filters, defaultFilters());
    selectedProcessPid.value = null;
    replayPath.value = '';
    await applyFilters();
  };

  const focusProcess = async (pid: number | null) => {
    selectedProcessPid.value = pid;
    filters.pid = pid ? String(pid) : '';
    filters.processTree = Boolean(pid);
    if (pid && filters.timePreset === 'all') {
      filters.timePreset = '24h';
    }
    await applyFilters();
  };

  return {
    timePresetOptions,
    applyFilters,
    resetFilters,
    focusProcess,
  };
}
