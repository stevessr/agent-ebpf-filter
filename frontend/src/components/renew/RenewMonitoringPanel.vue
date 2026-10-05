<script setup lang="ts">
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
import type { CollectorHealthResponse } from "../../types/config";

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
  collectorHealth: Partial<CollectorHealthResponse> | null;
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

const runtimeModules: Array<{
  key: RenewRuntimeToggleKey;
  title: string;
  description: string;
  cost: RenewMonitoringCost;
  manual?: boolean;
}> = [
  {
    key: "loopDetection",
    title: "行为循环检测",
    description: "识别 Agent 重复执行、反复读写和资源浪费循环。",
    cost: "low",
  },
  {
    key: "signalProcessing",
    title: "行为信号分析",
    description: "把底层事件组合成更易处理的安全和行为信号。",
    cost: "medium",
  },
  {
    key: "researchProcessing",
    title: "研究分析管线",
    description: "生成时间线、Top-K、会话聚合等研究数据；日常模式默认关闭。",
    cost: "high",
  },
  {
    key: "tlsCapture",
    title: "TLS 明文深度捕获",
    description: "仅在排查协议或 Agent 流量时开启；高开销且涉及敏感明文。",
    cost: "high",
    manual: true,
  },
  {
    key: "persistence",
    title: "本地事件持久化",
    description:
      "默认开启；完整事件写入后端 Pebble 数据库（默认 25 万条 / 168h），Renew 只保留摘要。",
    cost: "medium",
    manual: true,
  },
];

const isModuleEnabled = (key: string) => props.enabledModuleKeys.includes(key);

const healthy = () =>
  Boolean(props.collectorHealth) &&
  props.collectorHealth?.captureHealthy !== false &&
  Number(props.collectorHealth?.ringbufDroppedTotal || 0) === 0;

const stat = (key: keyof CollectorHealthResponse) =>
  Number(props.collectorHealth?.[key] || 0);
</script>

<template>
  <div class="renew-monitoring">
    <header class="renew-monitoring__header">
      <div>
        <div class="renew-eyebrow">MONITORING CENTER</div>
        <h1>监控中心</h1>
        <p>
          按需启用监控层级。日常保留高价值信号，需要排查时再打开高频采集与研究处理。
        </p>
      </div>
      <button
        class="renew-icon-button"
        title="刷新状态"
        @click="emit('refresh')"
      >
        <ReloadOutlined />
      </button>
    </header>

    <section class="renew-monitoring__summary">
      <div>
        <span class="renew-monitoring__label">预计附加开销</span>
        <strong :class="`is-${overhead.tone}`">{{
          ready ? overhead.label : "未加载"
        }}</strong>
      </div>
      <div>
        <span class="renew-monitoring__label">采集健康</span>
        <strong :class="healthy() ? 'is-low' : 'is-high'">
          {{ !collectorHealth ? "未获取" : healthy() ? "正常" : "有丢弃" }}
        </strong>
      </div>
      <div>
        <span class="renew-monitoring__label">Ringbuf 丢弃</span>
        <strong>{{ stat("ringbufDroppedTotal") }}</strong>
      </div>
      <div>
        <span class="renew-monitoring__label">后端队列</span>
        <strong>{{ stat("backendQueueLen") }}</strong>
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
          <h2>监控档位</h2>
          <p>快速切换常驻开销；TLS 明文仍要求手动开启，持久化默认保持开启。</p>
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
            <strong>{{ profile.title }}</strong>
            <CheckCircleFilled v-if="activeProfileKey === profile.key" />
          </span>
          <span>{{ profile.description }}</span>
        </button>
      </div>
      <div
        v-if="activeProfileKey === 'custom'"
        class="renew-monitoring__custom"
      >
        当前为自定义组合。
      </div>
    </section>

    <section class="renew-panel">
      <div class="renew-panel__header">
        <div>
          <h2>实时行为监控</h2>
          <p>
            关闭事件组会减少后端解码后的分析、归档与推送开销；核心 eBPF
            探针仍保持挂载，避免切换时重载内核程序。
          </p>
        </div>
        <SafetyCertificateOutlined />
      </div>

      <div class="renew-monitor-grid">
        <article
          v-for="module in modules"
          :key="module.key"
          class="renew-monitor-card"
          :class="{
            'renew-monitor-card--enabled': isModuleEnabled(module.key),
          }"
        >
          <div class="renew-monitor-card__top">
            <div>
              <strong>{{ module.title }}</strong>
              <span :class="`is-${module.cost}`">{{
                costText[module.cost]
              }}</span>
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
          <h2>分析与深度采集</h2>
          <p>这些开关会直接关闭相应后端处理路径，节省队列、CPU、内存或 I/O。</p>
        </div>
        <ExperimentOutlined />
      </div>

      <div class="renew-runtime-list">
        <article
          v-for="item in runtimeModules"
          :key="item.key"
          class="renew-runtime-item"
        >
          <div>
            <div class="renew-runtime-item__title">
              <strong>{{ item.title }}</strong>
              <span :class="`is-${item.cost}`">{{ costText[item.cost] }}</span>
              <em v-if="item.manual">手动</em>
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

    <section class="renew-panel">
      <div class="renew-panel__header">
        <div>
          <h2>系统状态采样</h2>
          <p>调低刷新频率可以减少常驻桌面端与后端指标采集开销。</p>
        </div>
      </div>
      <a-segmented
        :value="statsIntervalMs"
        :options="[
          { label: '2 秒 · 深度', value: 2000 },
          { label: '5 秒 · 日常', value: 5000 },
          { label: '10 秒 · 轻量', value: 10000 },
          { label: '30 秒 · 极省', value: 30000 },
        ]"
        :disabled="applying || loading || !ready"
        @update:value="emit('updateStatsInterval', Number($event))"
      />
    </section>
  </div>
</template>
