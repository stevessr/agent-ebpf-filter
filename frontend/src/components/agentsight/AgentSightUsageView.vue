<script setup lang="ts">
import { computed } from "vue";
import type { AgentSightEvent } from "../../utils/agentsight";

// Aggregate only measured token metadata; plaintext capture is not required.
const props = defineProps<{ events: AgentSightEvent[] }>();

type RecordValue = Record<string, unknown>;
interface UsageRow {
  key: string;
  count: number;
  input: number;
  output: number;
  cacheRead: number;
  cacheCreation: number;
}
const asObject = (v: unknown): RecordValue =>
  v !== null && typeof v === "object" && !Array.isArray(v)
    ? (v as RecordValue)
    : {};
const firstText = (o: RecordValue, ...keys: string[]): string => {
  for (const key of keys) {
    const value = o[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
};
const numberField = (
  o: RecordValue,
  ...keys: string[]
): { found: boolean; value: number } => {
  for (const key of keys) {
    const raw = o[key];
    if (raw === undefined || raw === null) continue;
    if (typeof raw !== "number" && typeof raw !== "string") continue;
    if (typeof raw === "string" && !/^\d+$/.test(raw.trim())) continue;
    const value = Number(raw);
    if (Number.isSafeInteger(value) && value >= 0)
      return { found: true, value };
  }
  return { found: false, value: 0 };
};
const usageForEvent = (data: RecordValue) => {
  const envelope = asObject(data.envelope);
  const attributes = asObject(data.attributes);
  const candidates = [
    asObject(data.usage),
    asObject(data.token_usage),
    asObject(data.tokenUsage),
    asObject(data.llm_call),
    asObject(data.llmCall),
    asObject(envelope.llm_call),
    asObject(envelope.llmCall),
    attributes,
    data,
  ];
  let model =
    firstText(data, "model", "model_name", "modelName") ||
    firstText(attributes, "gen_ai.response.model", "gen_ai.request.model");
  for (const candidate of candidates) {
    const input = numberField(candidate, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens", "gen_ai.usage.input_tokens");
    const output = numberField(candidate, "output_tokens", "outputTokens", "completion_tokens", "completionTokens", "gen_ai.usage.output_tokens");
    const cacheRead = numberField(candidate, "cache_read_input_tokens", "cacheReadInputTokens", "cached_tokens", "cachedTokens");
    const cacheCreation = numberField(candidate, "cache_creation_input_tokens", "cacheCreationInputTokens");
    if (!(input.found || output.found || cacheRead.found || cacheCreation.found))
      continue;
    model ||= firstText(candidate, "model", "model_name", "modelName");
    return { input: input.value, output: output.value, cacheRead: cacheRead.value, cacheCreation: cacheCreation.value, model: model || "unknown" };
  }
  return null;
};
const zeroRow = (key: string): UsageRow => ({
  key,
  count: 0,
  input: 0,
  output: 0,
  cacheRead: 0,
  cacheCreation: 0,
});
const summary = computed(() => {
  const sessions = new Map<string, UsageRow>();
  const models = new Map<string, UsageRow>();
  const seenCalls = new Set<string>();
  let attributed = 0;
  let measured = 0;
  const totals = zeroRow("total");
  for (const event of props.events) {
    const data = asObject(event.data);
    const envelope = asObject(data.envelope);
    const session =
      firstText(data, "agent_run_id", "agentRunId", "session_id", "sessionId") ||
      firstText(envelope, "agent_run_id", "agentRunId", "session_id", "sessionId") ||
      (event.traceId ? `trace:${event.traceId}` : "");
    if (session) {
      attributed += 1;
      if (!sessions.has(session)) sessions.set(session, zeroRow(session));
    }
    const usage = usageForEvent(data);
    if (!usage) continue;
    const callID =
      (event.spanId ? `span:${event.spanId}` : "") ||
      firstText(data, "response_id", "responseId", "llm_call_id", "llmCallId");
    if (callID) {
      const unique = `${session}|${callID}`;
      if (seenCalls.has(unique)) continue;
      seenCalls.add(unique);
    }
    measured += 1;
    const apply = (row: UsageRow) => {
      row.count += 1;
      row.input += usage.input;
      row.output += usage.output;
      row.cacheRead += usage.cacheRead;
      row.cacheCreation += usage.cacheCreation;
    };
    apply(totals);
    if (session) apply(sessions.get(session)!);
    if (!models.has(usage.model)) models.set(usage.model, zeroRow(usage.model));
    apply(models.get(usage.model)!);
  }
  return {
    totalEvents: props.events.length,
    attributed,
    measured,
    totals,
    sessions: [...sessions.values()].sort((a, b) => b.input - a.input),
    models: [...models.values()].sort((a, b) => b.input - a.input),
  };
});
const columns = [
  { title: "模型 / 会话", dataIndex: "key", key: "key", ellipsis: true },
  { title: "调用证据", dataIndex: "count", key: "count" },
  { title: "输入", dataIndex: "input", key: "input" },
  { title: "输出", dataIndex: "output", key: "output" },
  { title: "缓存读取", dataIndex: "cacheRead", key: "cacheRead" },
  { title: "缓存创建", dataIndex: "cacheCreation", key: "cacheCreation" },
];
</script>

<template>
  <div class="agentsight-usage">
    <a-alert
      type="info"
      show-icon
      message="AgentSight · Sessions & Token Usage"
      description="仅汇总事件中明确提供的 Token 计数。未采集到用量的调用属于未知值，而非 0 Token；会话标识来自显式的 agent_run_id / session_id / trace_id。"
      style="margin-bottom: 14px"
    />
    <a-row :gutter="[12, 12]" style="margin-bottom: 16px">
      <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="输入 Token" :value="summary.totals.input" /></a-card></a-col>
      <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="输出 Token" :value="summary.totals.output" /></a-card></a-col>
      <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="有用量证据的事件" :value="summary.measured" :suffix="`/ ${summary.totalEvents}`" /></a-card></a-col>
      <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="已归因事件" :value="summary.attributed" :suffix="`/ ${summary.totalEvents}`" /></a-card></a-col>
    </a-row>
    <a-card title="按模型统计" size="small" style="margin-bottom: 14px">
      <a-table :data-source="summary.models" :columns="columns" row-key="key" size="small" :pagination="{ pageSize: 8 }" />
    </a-card>
    <a-card title="按 Agent 会话统计" size="small">
      <a-table :data-source="summary.sessions" :columns="columns" row-key="key" size="small" :pagination="{ pageSize: 8 }" />
    </a-card>
  </div>
</template>
