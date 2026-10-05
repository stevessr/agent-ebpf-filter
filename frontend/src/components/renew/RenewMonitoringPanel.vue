<script setup lang="ts">
import { computed } from "vue";
import {
  CheckCircleFilled,
  ExperimentOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons-vue";

import type {
  RenewMonitoringCost,
  RenewMonitoringModule,
  RenewMonitoringProfile,
  RenewMonitoringProfileKey,
} from "../../composables/renew/monitoringPresets";
import type { RenewRuntimeToggleKey } from "../../composables/renew/useRenewMonitoringControls";
import type {
  CollectorHealthResponse,
  RuntimeSettings,
} from "../../types/config";

import RenewSwitch from "./RenewSwitch.vue";

const props = defineProps<{
  modules: RenewMonitoringModule[];
  profiles: RenewMonitoringProfile[];
  enabledModuleKeys: string[];
  activeProfileKey: RenewMonitoringProfileKey | "custom";
  statsIntervalMs: number;
  applying: boolean;
  loading: boolean;
  ready: boolean;
  error: string;
  overhead: { label: string; tone: RenewMonitoringCost };
  runtimeEnabled: (key: RenewRuntimeToggleKey) => boolean;
  runtimeSettings: RuntimeSettings | null;
  collectorHealth: Partial<CollectorHealthResponse> | null;
  persistedEventLogPath: string;
  persistedEventLogAlive: boolean;
}>();

const emit = defineEmits<{
  applyProfile: [key: RenewMonitoringProfileKey];
  toggleModule: [key: string, enabled: boolean];
  toggleRuntime: [key: RenewRuntimeToggleKey, enabled: boolean];
  updateStatsInterval: [value: number];
  refresh: [];
}>();

const costText: Record<RenewMonitoringCost, string> = {
  low: "低开销",
  medium: "中等",
  high: "高开销",
};

const coreModuleKeys = new Set(["process", "file-changes", "network"]);

const coreModules = computed(() =>
  props.modules.filter((module) => coreModuleKeys.has(module.key)),
);

const advancedModules = computed(() =>
  props.modules.filter((module) => !coreModuleKeys.has(module.key)),
);

const analysisModules: Array<{
  key: RenewRuntimeToggleKey;
  title: string;
  description: string;
  cost: RenewMonitoringCost;
  manual?: boolean;
}> = [
  {
    key: "loopDetection",
    title: "行为循环检测",
    description: "识别 Agent 重复执行、反复读写与资源浪费循环。",
    cost: "low",
  },
  {
    key: "signalProcessing",
    title: "行为信号分析",
    description: "将底层事件归并为可读的行为与安全信号。",
    cost: "medium",
  },
  {
    key: "researchProcessing",
    title: "研究分析管线",
    description: "生成时间线、Top-K 与会话聚合；适合短时调查。",
    cost: "high",
  },
  {
    key: "tlsCapture",
    title: "TLS 明文深度捕获",
    description: "仅在排查 Agent 协议流量时开启，会接触敏感明文。",
    cost: "high",
    manual: true,
  },
];

const isModuleEnabled = (key: string) => props.enabledModuleKeys.includes(key);

const coreEnabledCount = computed(
  () => coreModules.value.filter((module) => isModuleEnabled(module.key)).length,
);

const analysisEnabledCount = computed(
  () => analysisModules.filter((item) => props.runtimeEnabled(item.key)).length,
);

const activeProfileTitle = computed(
  () =>
    props.profiles.find((profile) => profile.key === props.activeProfileKey)
      ?.title || "自定义",
);

const healthy = computed(
  () =>
    Boolean(props.collectorHealth) &&
    props.collectorHealth?.captureHealthy !== false &&
    Number(props.collectorHealth?.ringbufDroppedTotal || 0) === 0 &&
    Number(props.collectorHealth?.ringbufReserveFailedTotal || 0) === 0,
);

const persistenceEnabled = computed(() => props.runtimeEnabled("persistence"));

const persistenceHealthy = computed(
  () =>
    persistenceEnabled.value &&
    (props.persistedEventLogAlive ||
      props.collectorHealth?.persistWriterActive === true),
);

const statusTitle = computed(() => {
  if (!props.ready) return "正在加载监控配置";
  if (!healthy.value) return "监控链路需要关注";
  if (props.activeProfileKey === "deep") return "深度监控已开启";
  if (props.activeProfileKey === "daily") return "日常防护运行中";
  if (props.activeProfileKey === "lite") return "轻量监控运行中";
  return "自定义监控运行中";
});

const statusDescription = computed(() => {
  if (!props.ready) return "正在读取后端确认的监控状态。";
  if (!healthy.value) {
    return "采集端出现丢弃或缓冲区预留失败，建议先检查负载，再决定是否降低监控档位。";
  }
  if (props.activeProfileKey === "deep") {
    return "所有事件组与研究处理均已启用，适合短时排查；长期常驻建议切回日常模式。";
  }
  if (props.activeProfileKey === "daily") {
    return "核心进程、文件改动与基础联网保持监控，高频事件按需开启。";
  }
  if (props.activeProfileKey === "lite") {
    return "仅保留最关键的进程与文件改动监控，适合资源敏感环境。";
  }
  return "当前使用自定义组合；各模块状态会持久化并在重启后继续生效。";
});

const stat = (key: keyof CollectorHealthResponse) =>
  Number(props.collectorHealth?.[key] || 0);

const formatCount = (value: number) => {
  if (value >= 100_000 && value % 10_000 === 0) {
    return `${value / 10_000} 万`;
  }
  return value.toLocaleString();
};

const retentionAge = computed(() => {
  const raw = props.runtimeSettings?.eventStoreMaxAge || "";
  const match = /^(\d+)h$/.exec(raw);
  if (match && Number(match[1]) % 24 === 0) {
    return `${Number(match[1]) / 24} 天`;
  }
  return raw || "未限制";
});

const retentionLabel = computed(() => {
  const records = Number(props.runtimeSettings?.eventStoreMaxRecords || 0);
  const count = records > 0 ? formatCount(records) : "未限制";
  return `${count} / ${retentionAge.value}`;
});

const persistenceStateText = computed(() => {
  if (!persistenceEnabled.value) return "已关闭";
  return persistenceHealthy.value ? "运行中" : "需要检查";
});

const persistenceTone = computed<RenewMonitoringCost>(() => {
  if (!persistenceEnabled.value) return "medium";
  return persistenceHealthy.value ? "low" : "high";
});

const persistQueueUsage = computed(() => {
  const cap = stat("persistQueueCap");
  if (cap <= 0) return 0;
  return Math.min(100, Math.round((stat("persistQueueLen") / cap) * 100));
});
</script>

<template>
  <div class="renew-monitoring">
    <header class="renew-monitoring__header">
      <div>
        <div class="renew-eyebrow">MONITORING CENTER</div>
        <h1>监控中心</h1>
        <p>
          将日常防护与深度调查分开：常驻只保留高价值监控，需要时再打开高频采集和研究能力。
        </p>
      </div>
      <button class="renew-icon-button" title="刷新状态" @click="emit('refresh')">
        <ReloadOutlined />
      </button>
    </header>

    <section
      class="renew-protection-hero"
      :class="{ 'renew-protection-hero--warning': !healthy }"
    >
      <div class="renew-protection-hero__mark">
        <SafetyCertificateOutlined />
      </div>
      <div class="renew-protection-hero__body">
        <div class="renew-protection-hero__eyebrow">
          <span :class="healthy ? 'is-healthy' : 'is-warning'">
            {{ healthy ? "采集链路正常" : "采集链路异常" }}
          </span>
          <span>{{ activeProfileTitle }}</span>
        </div>
        <h2>{{ statusTitle }}</h2>
        <p>{{ statusDescription }}</p>
        <div class="renew-protection-hero__chips">
          <span>{{ coreEnabledCount }}/{{ coreModules.length }} 核心监控</span>
          <span>{{ analysisEnabledCount }}/{{ analysisModules.length }} 分析能力</span>
          <span :class="`is-${overhead.tone}`">预计开销 {{ overhead.label }}</span>
          <span :class="`is-${persistenceTone}`">日志 {{ persistenceStateText }}</span>
        </div>
      </div>
      <div class="renew-protection-hero__actions">
        <button
          v-if="activeProfileKey !== 'daily'"
          class="renew-monitoring__primary-action"
          :disabled="applying || loading || !ready"
          @click="emit('applyProfile', 'daily')"
        >
          恢复日常推荐
        </button>
        <small>配置会自动保存</small>
      </div>
    </section>

    <section class="renew-monitoring__summary">
      <div>
        <span class="renew-monitoring__label">核心防护</span>
        <strong>{{ coreEnabledCount }} / {{ coreModules.length }}</strong>
        <small>进程、文件改动、基础联网</small>
      </div>
      <div>
        <span class="renew-monitoring__label">采集健康</span>
        <strong :class="healthy ? 'is-low' : 'is-high'">
          {{ healthy ? "正常" : "有丢弃" }}
        </strong>
        <small>
          dropped {{ stat("ringbufDroppedTotal") }} · reserve
          {{ stat("ringbufReserveFailedTotal") }}
        </small>
      </div>
      <div>
        <span class="renew-monitoring__label">后端队列</span>
        <strong>{{ stat("backendQueueLen") }}</strong>
        <small>持久化队列 {{ stat("persistQueueLen") }}/{{ stat("persistQueueCap") }}</small>
      </div>
      <div>
        <span class="renew-monitoring__label">日志历史</span>
        <strong :class="`is-${persistenceTone}`">{{ persistenceStateText }}</strong>
        <small>{{ retentionLabel }}</small>
      </div>
    </section>

    <a-alert
      v-if="error"
      type="error"
      show-icon
      :message="error"
      class="renew-monitoring__alert"
    />

    <section class="renew-panel renew-monitoring__profiles">
      <div class="renew-panel__header">
        <div>
          <h2>一键监控档位</h2>
          <p>快速切换长期常驻与短时排查所需的采集深度。</p>
        </div>
        <ThunderboltOutlined />
      </div>
      <div class="renew-profile-grid">
        <button
          v-for="profile in profiles"
          :key="profile.key"
          class="renew-profile"
          :class="{ 'renew-profile--active': activeProfileKey === profile.key }"
          :disabled="applying || loading || !ready"
          @click="emit('applyProfile', profile.key)"
        >
          <span class="renew-profile__top">
            <span>
              <strong>{{ profile.title }}</strong>
              <em v-if="profile.key === 'daily'">推荐</em>
            </span>
            <CheckCircleFilled v-if="activeProfileKey === profile.key" />
          </span>
          <span>{{ profile.description }}</span>
          <small>
            {{ profile.modules.length }} 组采集 · {{ profile.statsIntervalMs / 1000 }} 秒状态采样
          </small>
        </button>
      </div>
      <div v-if="activeProfileKey === 'custom'" class="renew-monitoring__custom">
        当前为自定义组合，不会被自动覆盖。
      </div>
    </section>

    <section class="renew-panel">
      <div class="renew-panel__header">
        <div>
          <h2>核心防护</h2>
          <p>适合长期保持开启的低开销监控，是日常推荐模式的基础。</p>
        </div>
        <SafetyCertificateOutlined />
      </div>

      <div class="renew-monitor-grid renew-monitor-grid--core">
        <article
          v-for="module in coreModules"
          :key="module.key"
          class="renew-monitor-card"
          :class="{ 'renew-monitor-card--enabled': isModuleEnabled(module.key) }"
        >
          <div class="renew-monitor-card__top">
            <div>
              <span
                class="renew-monitor-card__state"
                :class="{ 'is-enabled': isModuleEnabled(module.key) }"
              />
              <strong>{{ module.title }}</strong>
              <em>建议常开</em>
            </div>
            <RenewSwitch
              :checked="isModuleEnabled(module.key)"
              :label="module.title"
              :disabled="applying || loading || !ready"
              @update:checked="emit('toggleModule', module.key, $event)"
            />
          </div>
          <p>{{ module.description }}</p>
          <small>{{ module.coverage }}</small>
        </article>
      </div>
    </section>

    <section class="renew-panel renew-panel--subtle">
      <div class="renew-panel__header">
        <div>
          <h2>按需深度监控</h2>
          <p>高频事件会增加解码、归档和分析量，建议仅在定位问题时短时开启。</p>
        </div>
        <ExperimentOutlined />
      </div>

      <div class="renew-monitor-grid">
        <article
          v-for="module in advancedModules"
          :key="module.key"
          class="renew-monitor-card renew-monitor-card--advanced"
          :class="{ 'renew-monitor-card--enabled': isModuleEnabled(module.key) }"
        >
          <div class="renew-monitor-card__top">
            <div>
              <span
                class="renew-monitor-card__state"
                :class="{ 'is-enabled': isModuleEnabled(module.key) }"
              />
              <strong>{{ module.title }}</strong>
              <span :class="`is-${module.cost}`">{{ costText[module.cost] }}</span>
            </div>
            <RenewSwitch
              :checked="isModuleEnabled(module.key)"
              :label="module.title"
              :disabled="applying || loading || !ready"
              @update:checked="emit('toggleModule', module.key, $event)"
            />
          </div>
          <p>{{ module.description }}</p>
          <small>{{ module.coverage }}</small>
        </article>
      </div>
    </section>

    <section class="renew-panel">
      <div class="renew-panel__header">
        <div>
          <h2>行为分析</h2>
          <p>这些开关会真正停掉对应的后端分析路径，而不仅是隐藏前端结果。</p>
        </div>
        <ExperimentOutlined />
      </div>

      <div class="renew-runtime-list">
        <article
          v-for="item in analysisModules"
          :key="item.key"
          class="renew-runtime-item"
          :class="{ 'renew-runtime-item--enabled': runtimeEnabled(item.key) }"
        >
          <div>
            <div class="renew-runtime-item__title">
              <span
                class="renew-monitor-card__state"
                :class="{ 'is-enabled': runtimeEnabled(item.key) }"
              />
              <strong>{{ item.title }}</strong>
              <span :class="`is-${item.cost}`">{{ costText[item.cost] }}</span>
              <em v-if="item.manual">手动开启</em>
            </div>
            <p>{{ item.description }}</p>
          </div>
          <RenewSwitch
            :checked="runtimeEnabled(item.key)"
            :label="item.title"
            :disabled="applying || loading || !ready"
            @update:checked="emit('toggleRuntime', item.key, $event)"
          />
        </article>
      </div>
    </section>

    <section class="renew-monitoring__bottom-grid">
      <article class="renew-panel renew-storage-card">
        <div class="renew-panel__header">
          <div>
            <h2>日志与历史</h2>
            <p>完整事件只保存在后端，Renew 仅按需读取摘要与单条详情。</p>
          </div>
          <RenewSwitch
            :checked="runtimeEnabled('persistence')"
            label="本地事件持久化"
            :disabled="applying || loading || !ready"
            @update:checked="emit('toggleRuntime', 'persistence', $event)"
          />
        </div>

        <div class="renew-storage-card__status">
          <div>
            <span class="renew-monitoring__label">Pebble writer</span>
            <strong :class="`is-${persistenceTone}`">{{ persistenceStateText }}</strong>
          </div>
          <div>
            <span class="renew-monitoring__label">保留策略</span>
            <strong>{{ retentionLabel }}</strong>
          </div>
        </div>

        <div class="renew-storage-card__path">
          <span>数据库路径</span>
          <code>{{ persistedEventLogPath || runtimeSettings?.logFilePath || "—" }}</code>
        </div>

        <div class="renew-storage-card__queue">
          <span>
            写入队列 {{ stat("persistQueueLen") }}/{{ stat("persistQueueCap") }}
          </span>
          <span>{{ persistQueueUsage }}%</span>
        </div>
        <div class="renew-storage-card__meter">
          <span :style="{ width: `${persistQueueUsage}%` }" />
        </div>
      </article>

      <article class="renew-panel renew-sampling-card">
        <div class="renew-panel__header">
          <div>
            <h2>状态采样频率</h2>
            <p>只影响桌面状态刷新与指标采集，不改变已启用的事件类型。</p>
          </div>
        </div>

        <div class="renew-sampling-card__current">
          <strong>{{ statsIntervalMs / 1000 }} 秒</strong>
          <span>当前刷新间隔</span>
        </div>

        <a-segmented
          :value="statsIntervalMs"
          :options="[
            { label: '2 秒', value: 2000 },
            { label: '5 秒', value: 5000 },
            { label: '10 秒', value: 10000 },
            { label: '30 秒', value: 30000 },
          ]"
          :disabled="applying || loading || !ready"
          block
          @update:value="emit('updateStatsInterval', Number($event))"
        />

        <p class="renew-sampling-card__hint">
          日常推荐 5 秒；深度排查可临时调整为 2 秒，后台常驻可提高到 10–30 秒。
        </p>
      </article>
    </section>
  </div>
</template>
