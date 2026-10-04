<script setup lang="ts">
import {
  AlertOutlined,
  AppstoreOutlined,
  ArrowRightOutlined,
  CheckCircleFilled,
  DashboardOutlined,
  GlobalOutlined,
  NodeIndexOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
} from "@ant-design/icons-vue";

import { useRenewDashboard } from "../../composables/renew/useRenewDashboard";
import "./renew.css";

const {
  events,
  isConnected,
  isPaused,
  processes,
  systemStats,
  trackedProcesses,
  search,
  onlyAgents,
  recentEvents,
  attentionEvents,
  attentionCount,
  blockedCount,
  agentEventCount,
  activeAgents,
  topProcesses,
  topDestinations,
  connectionLabel,
  alertTone,
  formatBytesWithUnit,
  formatRate,
  describeEvent,
  eventTime,
  eventTone,
  eventLabel,
  go,
} = useRenewDashboard();
</script>

<template>
  <div class="renew-shell">
    <aside class="renew-sidebar">
      <div class="renew-brand">
        <div class="renew-brand__mark">R</div>
        <div>
          <strong>Renew</strong>
          <span>Agent Monitor</span>
        </div>
      </div>

      <nav class="renew-nav" aria-label="Renew navigation">
        <button class="renew-nav__item renew-nav__item--active">
          <DashboardOutlined />
          <span>概览</span>
        </button>
        <button class="renew-nav__item" @click="go('Dashboard')">
          <AppstoreOutlined />
          <span>事件</span>
        </button>
        <button class="renew-nav__item" @click="go('NetworkFlow', { tab: 'overview' })">
          <GlobalOutlined />
          <span>网络</span>
        </button>
        <button class="renew-nav__item" @click="go('Monitor', { tab: 'processes' })">
          <NodeIndexOutlined />
          <span>进程</span>
        </button>
        <button class="renew-nav__item" @click="go('Config', { tab: 'security' })">
          <SafetyCertificateOutlined />
          <span>规则</span>
        </button>
      </nav>

      <div class="renew-sidebar__footer">
        <button class="renew-nav__item" @click="go('Config', { tab: 'runtime' })">
          <SettingOutlined />
          <span>设置</span>
        </button>
        <button class="renew-professional" @click="go('Dashboard')">
          打开专业工作台
          <ArrowRightOutlined />
        </button>
      </div>
    </aside>

    <main class="renew-main">
      <header class="renew-header">
        <div>
          <div class="renew-eyebrow">DAILY MONITORING</div>
          <h1>今天的 Agent 活动</h1>
          <p>把内核事件整理成日常可读的状态、异常和最近动作。</p>
        </div>
        <div class="renew-header__actions">
          <div class="renew-live" :class="{ 'renew-live--offline': !isConnected }">
            <span class="renew-live__dot" />
            {{ connectionLabel }}
          </div>
          <button
            class="renew-icon-button"
            :title="isPaused ? '继续事件流' : '暂停事件流'"
            @click="isPaused = !isPaused"
          >
            <PlayCircleOutlined v-if="isPaused" />
            <PauseCircleOutlined v-else />
          </button>
        </div>
      </header>

      <section class="renew-metrics" aria-label="System overview">
        <article class="renew-metric">
          <div class="renew-metric__label">采集状态</div>
          <div class="renew-metric__value">
            {{ isConnected ? "Online" : "Offline" }}
          </div>
          <div class="renew-metric__meta">
            当前缓冲区 {{ events.length }} 条 · Agent {{ agentEventCount }} 条
          </div>
        </article>

        <article class="renew-metric">
          <div class="renew-metric__label">CPU</div>
          <div class="renew-metric__value">
            {{ systemStats.cpuTotal.toFixed(1) }}%
          </div>
          <div class="renew-meter">
            <span :style="{ width: Math.min(100, systemStats.cpuTotal) + '%' }" />
          </div>
          <div class="renew-metric__meta">{{ processes.length }} 个进程</div>
        </article>

        <article class="renew-metric">
          <div class="renew-metric__label">内存</div>
          <div class="renew-metric__value">
            {{ systemStats.memPercent.toFixed(1) }}%
          </div>
          <div class="renew-meter">
            <span :style="{ width: Math.min(100, systemStats.memPercent) + '%' }" />
          </div>
          <div class="renew-metric__meta">
            {{ formatBytesWithUnit(systemStats.memUsed) }} /
            {{ formatBytesWithUnit(systemStats.memTotal) }}
          </div>
        </article>

        <article class="renew-metric" :class="`renew-metric--${alertTone}`">
          <div class="renew-metric__label">需要关注</div>
          <div class="renew-metric__value">{{ attentionCount }}</div>
          <div class="renew-metric__meta">
            已阻断 {{ blockedCount }} ·
            {{ attentionCount ? "建议查看" : "暂时正常" }}
          </div>
        </article>
      </section>

      <section class="renew-toolbar">
        <a-input
          v-model:value="search"
          allow-clear
          placeholder="搜索 Agent、动作、文件或网络目标"
          class="renew-search"
        >
          <template #prefix><SearchOutlined /></template>
