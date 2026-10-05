<script setup lang="ts">
import { computed } from "vue";

import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import {
  buildRenewActivityBuckets,
  buildRenewCategorySlices,
  buildRenewSafetySlices,
} from "../../composables/renew/dashboardVisualization";

const props = defineProps<{
  events: AgentEvent[];
  connected: boolean;
}>();

const buckets = computed(() => buildRenewActivityBuckets(props.events));
const categories = computed(() => buildRenewCategorySlices(props.events));
const safety = computed(() => buildRenewSafetySlices(props.events));
const maxBucket = computed(() =>
  Math.max(1, ...buckets.value.map((bucket) => bucket.count)),
);
const recentCount = computed(() =>
  buckets.value.reduce((sum, bucket) => sum + bucket.count, 0),
);
const recentAttention = computed(() =>
  buckets.value.reduce((sum, bucket) => sum + bucket.attention, 0),
);
const dangerCount = computed(
  () => safety.value.find((item) => item.key === "danger")?.count || 0,
);
const warningCount = computed(
  () => safety.value.find((item) => item.key === "warning")?.count || 0,
);
const status = computed(() => {
  if (!props.connected) {
    return {
      tone: "offline",
      label: "采集端离线",
      summary: "当前图表显示已加载的本地历史摘要。",
    };
  }
  if (dangerCount.value > 0) {
    return {
      tone: "danger",
      label: "有高风险活动",
      summary: `当前摘要窗口有 ${dangerCount.value} 条高风险或阻断记录。`,
    };
  }
  if (warningCount.value > 0) {
    return {
      tone: "warning",
      label: "有事件需要关注",
      summary: `当前摘要窗口有 ${warningCount.value} 条需要关注的记录。`,
    };
  }
  return {
    tone: "normal",
    label: "运行正常",
    summary: "当前已加载摘要中没有需要处理的异常。",
  };
});
</script>

<template>
  <section class="renew-panel renew-visual-overview">
    <div class="renew-panel__header renew-visual-overview__header">
      <div>
        <h2>活动概览</h2>
        <p>把底层事件汇总成趋势、行为类型和安全结果，先看整体是否正常。</p>
      </div>
      <div class="renew-visual-status" :class="`is-${status.tone}`">
        <strong>{{ status.label }}</strong>
        <span>{{ status.summary }}</span>
      </div>
    </div>

    <div class="renew-visual-grid">
      <article class="renew-visual-card renew-visual-card--timeline">
        <div class="renew-visual-card__title">
          <div>
            <strong>最近 60 分钟</strong>
            <span>{{ recentCount }} 条活动 · {{ recentAttention }} 条需关注</span>
          </div>
          <span>每 5 分钟</span>
        </div>

        <div
          class="renew-activity-chart"
          role="img"
          :aria-label="`最近 60 分钟共 ${recentCount} 条事件，其中 ${recentAttention} 条需要关注`"
        >
          <div
            v-for="bucket in buckets"
            :key="bucket.key"
            class="renew-activity-chart__column"
            :title="`${bucket.label} · ${bucket.count} 条 · 关注 ${bucket.attention}`"
          >
            <div class="renew-activity-chart__track">
              <div
                class="renew-activity-chart__bar"
                :class="{ 'has-attention': bucket.attention > 0 }"
                :style="{
                  height:
                    bucket.count === 0
                      ? '3px'
                      : Math.max(8, (bucket.count / maxBucket) * 100) + '%',
                }"
              >
                <span
                  v-if="bucket.attention"
                  class="renew-activity-chart__attention"
                  :style="{
                    height:
                      Math.max(
                        18,
                        (bucket.attention / Math.max(1, bucket.count)) * 100,
                      ) + '%',
                  }"
                />
              </div>
            </div>
            <span>{{ bucket.label }}</span>
          </div>
        </div>
      </article>

      <article class="renew-visual-card">
        <div class="renew-visual-card__title">
          <div>
            <strong>行为构成</strong>
            <span>按当前已加载摘要分类</span>
          </div>
        </div>

        <div class="renew-category-chart">
          <div
            v-for="item in categories"
            :key="item.key"
            class="renew-category-chart__row"
          >
            <span>{{ item.label }}</span>
            <div class="renew-category-chart__track">
              <span
                :class="`is-${item.key}`"
                :style="{ width: Math.max(item.count ? 3 : 0, item.share) + '%' }"
              />
            </div>
            <strong>{{ item.count }}</strong>
          </div>
        </div>
      </article>

      <article class="renew-visual-card">
        <div class="renew-visual-card__title">
          <div>
            <strong>安全结果</strong>
            <span>普通 / 需关注 / 高风险</span>
          </div>
        </div>

        <div class="renew-safety-chart">
          <div class="renew-safety-chart__bar" aria-hidden="true">
            <span
              v-for="item in safety"
              :key="item.key"
              :class="`is-${item.key}`"
              :style="{ width: item.share + '%' }"
            />
          </div>
          <div class="renew-safety-chart__legend">
            <div v-for="item in safety" :key="item.key">
              <span class="renew-safety-chart__dot" :class="`is-${item.key}`" />
              <span>{{ item.label }}</span>
              <strong>{{ item.count }}</strong>
            </div>
          </div>
        </div>
      </article>
    </div>

    <p class="renew-visual-overview__note">
      图表统计当前浏览器已加载的紧凑摘要窗口；完整历史仍保存在后端事件库中，按需加载，不会把全部 JSON 常驻在 WebUI 内存。
    </p>
  </section>
</template>

<style scoped>
.renew-visual-overview {
  margin: 14px 0 18px;
}

.renew-visual-overview__header {
  align-items: center;
}

.renew-visual-status {
  min-width: min(300px, 42vw);
  border: 1px solid #dfe7e2;
  border-radius: 12px;
  padding: 9px 12px;
  display: grid;
  gap: 2px;
  background: #f8fbf9;
}

.renew-visual-status strong {
  color: #2f744d;
  font-size: 11px;
}

.renew-visual-status span {
  color: #758078;
  font-size: 9px;
}

.renew-visual-status.is-warning {
  border-color: #eee0c5;
  background: #fffaf1;
}

.renew-visual-status.is-warning strong {
  color: #9f6b16;
}

.renew-visual-status.is-danger {
  border-color: #f0d2d2;
  background: #fff7f7;
}

.renew-visual-status.is-danger strong {
  color: #b83d3d;
}

.renew-visual-status.is-offline {
  border-color: #e3e5e9;
  background: #f8f9fa;
}

.renew-visual-status.is-offline strong {
  color: #69717d;
}

.renew-visual-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(250px, 0.8fr) minmax(230px, 0.7fr);
  gap: 12px;
}

.renew-visual-card {
  min-width: 0;
  border: 1px solid #eceef2;
  border-radius: 12px;
  padding: 14px;
  background: #fbfbfc;
}

.renew-visual-card__title {
  min-height: 36px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}

.renew-visual-card__title > div {
  display: grid;
  gap: 2px;
}

.renew-visual-card__title strong {
  color: #343941;
  font-size: 11px;
}

.renew-visual-card__title span {
  color: #90959e;
  font-size: 9px;
}

.renew-activity-chart {
  height: 132px;
  margin-top: 8px;
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 5px;
  align-items: end;
}

.renew-activity-chart__column {
  min-width: 0;
  height: 100%;
  display: grid;
  grid-template-rows: minmax(0, 1fr) 16px;
  gap: 5px;
  align-items: end;
  text-align: center;
}

.renew-activity-chart__track {
  height: 100%;
  display: flex;
  align-items: flex-end;
  border-bottom: 1px solid #e9ebef;
}

.renew-activity-chart__bar {
  position: relative;
  width: 100%;
  min-height: 3px;
  overflow: hidden;
  border-radius: 4px 4px 1px 1px;
  background: #aeb6c1;
  transition: height 0.2s ease;
}

.renew-activity-chart__bar.has-attention {
  background: #aeb6c1;
}

.renew-activity-chart__attention {
  position: absolute;
  inset: auto 0 0;
  background: #d59a35;
}

.renew-activity-chart__column > span {
  overflow: hidden;
  color: #999ea7;
  font-size: 7px;
  text-overflow: clip;
  white-space: nowrap;
}

.renew-category-chart {
  margin-top: 8px;
  display: grid;
  gap: 9px;
}

.renew-category-chart__row {
  display: grid;
  grid-template-columns: 70px minmax(0, 1fr) 28px;
  gap: 8px;
  align-items: center;
}

.renew-category-chart__row > span,
.renew-category-chart__row strong {
  color: #6f7681;
  font-size: 9px;
}

.renew-category-chart__row strong {
  color: #41464f;
  text-align: right;
}

.renew-category-chart__track {
  height: 6px;
  overflow: hidden;
  border-radius: 999px;
  background: #eceef1;
}

.renew-category-chart__track span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: #8e99a7;
  transition: width 0.2s ease;
}

.renew-category-chart__track .is-network {
  background: #758ca0;
}

.renew-category-chart__track .is-process {
  background: #8d879f;
}

.renew-category-chart__track .is-agent {
  background: #8b9b8f;
}

.renew-category-chart__track .is-alert {
  background: #c98a4c;
}

.renew-category-chart__track .is-other {
  background: #b1b4ba;
}

.renew-safety-chart {
  margin-top: 10px;
}

.renew-safety-chart__bar {
  height: 12px;
  overflow: hidden;
  border-radius: 999px;
  display: flex;
  background: #eceef1;
}

.renew-safety-chart__bar span {
  min-width: 0;
  transition: width 0.2s ease;
}

.renew-safety-chart__bar .is-normal,
.renew-safety-chart__dot.is-normal {
  background: #7ca68b;
}

.renew-safety-chart__bar .is-warning,
.renew-safety-chart__dot.is-warning {
  background: #d3a24c;
}

.renew-safety-chart__bar .is-danger,
.renew-safety-chart__dot.is-danger {
  background: #cf6262;
}

.renew-safety-chart__legend {
  margin-top: 13px;
  display: grid;
  gap: 9px;
}

.renew-safety-chart__legend > div {
  display: grid;
  grid-template-columns: 8px minmax(0, 1fr) auto;
  gap: 7px;
  align-items: center;
  color: #747a84;
  font-size: 9px;
}

.renew-safety-chart__legend strong {
  color: #3e434b;
}

.renew-safety-chart__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.renew-visual-overview__note {
  margin: 12px 0 0;
  color: #969ba4;
  font-size: 9px;
  line-height: 1.6;
}

@media (max-width: 1180px) {
  .renew-visual-grid {
    grid-template-columns: minmax(0, 1.3fr) minmax(260px, 0.7fr);
  }

  .renew-visual-card--timeline {
    grid-row: span 2;
  }
}

@media (max-width: 820px) {
  .renew-visual-overview__header {
    align-items: stretch;
    display: grid;
  }

  .renew-visual-status {
    min-width: 0;
  }

  .renew-visual-grid {
    grid-template-columns: 1fr;
  }

  .renew-visual-card--timeline {
    grid-row: auto;
  }
}

@media (max-width: 520px) {
  .renew-activity-chart {
    gap: 3px;
  }

  .renew-activity-chart__column > span {
    font-size: 6px;
  }

  .renew-category-chart__row {
    grid-template-columns: 64px minmax(0, 1fr) 24px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .renew-activity-chart__bar,
  .renew-category-chart__track span,
  .renew-safety-chart__bar span {
    transition: none;
  }
}
</style>
