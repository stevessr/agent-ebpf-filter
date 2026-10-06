<script setup lang="ts">
import { computed } from "vue";

import { presentRenewEventDetail } from "../../composables/renew/eventDetailPresentation";

const props = defineProps<{
  open: boolean;
  loading: boolean;
  eventId: string;
  detail: Record<string, unknown> | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

const presentation = computed(() => presentRenewEventDetail(props.detail));
const prettyDetail = computed(() =>
  props.detail ? JSON.stringify(props.detail, null, 2) : "",
);
const riskWidth = computed(() =>
  Math.max(0, Math.min(100, presentation.value?.riskScore || 0)),
);
</script>

<template>
  <a-drawer
    :open="open"
    width="min(760px, 94vw)"
    title="事件详情"
    placement="right"
    @close="emit('close')"
  >
    <div class="renew-detail-meta">
      <span>事件 ID</span>
      <code>{{ eventId || "—" }}</code>
    </div>

    <a-spin :spinning="loading">
      <div v-if="presentation" class="renew-detail-visual">
        <div v-if="presentation.error" class="renew-detail-error">
          {{ presentation.error }}
        </div>

        <section
          v-else
          class="renew-detail-hero"
          :class="`is-${presentation.tone}`"
        >
          <div class="renew-detail-hero__main">
            <div class="renew-detail-hero__chips">
              <span>{{ presentation.categoryLabel }}</span>
              <span :class="`is-${presentation.tone}`">
                {{ presentation.outcome }}
              </span>
            </div>
            <h2>{{ presentation.title }}</h2>
            <p>{{ presentation.subtitle }}</p>
            <div class="renew-detail-hero__meta">
              <span>{{ presentation.processLabel }}</span>
              <span>{{ presentation.timestamp }}</span>
            </div>
          </div>

          <div class="renew-detail-risk">
            <div>
              <span>风险</span>
              <strong>{{ presentation.riskLabel }}</strong>
            </div>
            <div class="renew-detail-risk__score">
              <strong>{{
                presentation.riskAvailable
                  ? presentation.riskScore.toFixed(0)
                  : "—"
              }}</strong>
              <span>{{ presentation.riskAvailable ? "/ 100" : "未评分" }}</span>
            </div>
            <div class="renew-detail-risk__track" aria-hidden="true">
              <span
                :class="`is-${presentation.tone}`"
                :style="{
                  width: (presentation.riskAvailable ? riskWidth : 0) + '%',
                }"
              />
            </div>
          </div>
        </section>

        <template v-if="!presentation.error">
          <section
            v-for="section in presentation.sections"
            :key="section.key"
            class="renew-detail-section"
          >
            <div class="renew-detail-section__heading">
              <h3>{{ section.title }}</h3>
              <p v-if="section.description">{{ section.description }}</p>
            </div>
            <dl class="renew-detail-fields">
              <div v-for="field in section.fields" :key="field.key">
                <dt>{{ field.label }}</dt>
                <dd :class="{ 'is-mono': field.mono }">{{ field.value }}</dd>
              </div>
            </dl>
          </section>

          <section
            v-if="presentation.envelopeFields.length"
            class="renew-detail-section"
          >
            <div class="renew-detail-section__heading">
              <h3>结构化负载</h3>
              <p>
                事件封装（Envelope）中的结构化 JSON 已展开为可读字段；兼容层中的
                legacy 事件不再重复展示。
              </p>
            </div>
            <dl class="renew-detail-fields renew-detail-fields--payload">
              <div
                v-for="field in presentation.envelopeFields"
                :key="field.key"
              >
                <dt>{{ field.label }}</dt>
                <dd :class="{ 'is-mono': field.mono }">{{ field.value }}</dd>
              </div>
            </dl>
          </section>
        </template>

        <details class="renew-detail-raw">
          <summary>原始 JSON · 高级排障</summary>
          <p>
            普通监控不需要阅读这里；仅在核对后端字段、协议或提交问题时展开。
          </p>
          <pre>{{ prettyDetail }}</pre>
        </details>

        <p class="renew-detail-retention-note">
          完整事件按需从后端本地事件库读取；关闭详情后 Renew
          不再保留这份完整负载。
        </p>
      </div>

      <a-empty v-else description="选择一条事件查看可视化详情" />
    </a-spin>
  </a-drawer>
</template>

<style scoped>
.renew-detail-visual {
  display: grid;
  gap: 14px;
}

.renew-detail-error {
  border: 1px solid var(--renew-danger-border);
  border-radius: 12px;
  padding: 14px;
  background: var(--renew-danger-soft);
  color: var(--renew-danger-strong);
  font-size: 12px;
}

.renew-detail-hero {
  border: 1px solid var(--renew-border);
  border-radius: 16px;
  padding: 18px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 150px;
  gap: 18px;
  background: var(--renew-panel-subtle);
}

.renew-detail-hero.is-warning {
  border-color: var(--renew-warning-border);
  background: var(--renew-warning-wash);
}

.renew-detail-hero.is-danger {
  border-color: var(--renew-danger-border);
  background: var(--renew-danger-soft);
}

.renew-detail-hero__main {
  min-width: 0;
}

.renew-detail-hero__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.renew-detail-hero__chips span {
  border-radius: 999px;
  padding: 3px 8px;
  background: var(--renew-track);
  color: var(--renew-muted);
  font-size: 9px;
  font-weight: 700;
}

.renew-detail-hero__chips span.is-normal {
  background: var(--renew-success-soft);
  color: var(--renew-success-strong);
}

.renew-detail-hero__chips span.is-warning {
  background: var(--renew-warning-soft);
  color: var(--renew-warning-strong);
}

.renew-detail-hero__chips span.is-danger {
  background: var(--renew-danger-soft);
  color: var(--renew-danger-strong);
}

.renew-detail-hero h2 {
  margin: 11px 0 5px;
  color: var(--renew-text);
  font-size: 21px;
  letter-spacing: -0.02em;
}

.renew-detail-hero p {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--renew-muted);
  font-size: 12px;
  line-height: 1.6;
}

.renew-detail-hero__meta {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  color: var(--renew-faint);
  font-size: 9px;
}

.renew-detail-risk {
  align-self: stretch;
  border-left: 1px solid var(--renew-border);
  padding-left: 18px;
  display: grid;
  align-content: center;
  gap: 9px;
}

.renew-detail-risk > div:first-child {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: var(--renew-faint);
  font-size: 9px;
}

.renew-detail-risk > div:first-child strong {
  color: var(--renew-muted);
  font-size: 10px;
}

.renew-detail-risk__score {
  display: flex;
  align-items: baseline;
  gap: 4px;
}

.renew-detail-risk__score strong {
  color: var(--renew-text-strong);
  font-size: 30px;
  line-height: 1;
}

.renew-detail-risk__score span {
  color: var(--renew-faint);
  font-size: 9px;
}

.renew-detail-risk__track {
  height: 6px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--renew-track);
}

.renew-detail-risk__track > span {
  display: block;
  min-width: 0;
  height: 100%;
  border-radius: inherit;
  background: var(--renew-success-mid);
}

.renew-detail-risk__track > span.is-warning {
  background: var(--renew-warning-mid);
}

.renew-detail-risk__track > span.is-danger {
  background: var(--renew-danger-mid);
}

.renew-detail-section {
  border: 1px solid var(--renew-track);
  border-radius: 13px;
  padding: 15px;
  background: var(--renew-panel);
}

.renew-detail-section__heading {
  margin-bottom: 11px;
}

.renew-detail-section__heading h3 {
  margin: 0;
  color: var(--renew-text-strong);
  font-size: 12px;
}

.renew-detail-section__heading p {
  margin: 4px 0 0;
  color: var(--renew-faint);
  font-size: 9px;
  line-height: 1.55;
}

.renew-detail-fields {
  margin: 0;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  border: 1px solid var(--renew-track);
  border-radius: 10px;
  background: var(--renew-track);
}

.renew-detail-fields > div {
  min-width: 0;
  padding: 9px 10px;
  display: grid;
  gap: 4px;
  background: var(--renew-panel-subtle);
}

.renew-detail-fields dt {
  color: var(--renew-faint);
  font-size: 8px;
}

.renew-detail-fields dd {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--renew-text-strong);
  font-size: 10px;
  line-height: 1.5;
}

.renew-detail-fields dd.is-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 9px;
}

.renew-detail-fields--payload {
  grid-template-columns: 1fr;
}

.renew-detail-raw {
  border: 1px dashed var(--renew-border);
  border-radius: 12px;
  padding: 11px 13px;
  background: var(--renew-panel-subtle);
}

.renew-detail-raw summary {
  color: var(--renew-muted);
  font-size: 10px;
  font-weight: 700;
  cursor: pointer;
}

.renew-detail-raw > p,
.renew-detail-retention-note {
  color: var(--renew-faint);
  font-size: 9px;
  line-height: 1.6;
}

.renew-detail-raw > p {
  margin: 8px 0 0;
}

.renew-detail-raw pre {
  max-height: 360px;
  margin: 10px 0 0;
  overflow: auto;
  border: 1px solid var(--renew-border);
  border-radius: 9px;
  padding: 11px;
  background: var(--renew-code-bg);
  color: var(--renew-text-strong);
  font-size: 9px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.renew-detail-retention-note {
  margin: 0 2px;
}

@media (max-width: 620px) {
  .renew-detail-hero {
    grid-template-columns: 1fr;
  }

  .renew-detail-risk {
    border-top: 1px solid var(--renew-border);
    border-left: 0;
    padding-top: 13px;
    padding-left: 0;
  }

  .renew-detail-fields {
    grid-template-columns: 1fr;
  }
}
</style>
