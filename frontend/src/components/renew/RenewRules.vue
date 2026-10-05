<script setup lang="ts">
import { useRenewRules } from "../../composables/renew/useRenewRules";

const actionLabels: Record<string, string> = {
  ALLOW: "允许",
  BLOCK: "阻断",
  ALERT: "告警",
  REWRITE: "重写",
};
const actionLabel = (value: string) => actionLabels[value] || value;
const {
  rules,
  busy,
  ready,
  error,
  comm,
  action,
  regex,
  replacement,
  priority,
  rewrite,
  editing,
  pendingDelete,
  load,
  edit,
  reset,
  save,
  remove,
} = useRenewRules();
</script>
<template>
  <div class="renew-explorer">
    <header class="renew-monitoring__header">
      <div>
        <div class="renew-eyebrow">RENEW · 策略规则</div>
        <h1>规则</h1>
        <p>agent-wrapper 命令策略；由已认证的后端保存和执行。</p>
      </div>
      <button class="renew-link" :disabled="busy" @click="load">
        刷新规则
      </button>
    </header>
    <p class="renew-scope-note">
      共享配置：规则按命令名匹配，不按 Agent 工具隔离。上方 Agent 工具筛选不改变规则作用域；仅通过 agent-wrapper 执行的命令受此策略约束。
    </p>
    <p v-if="error" role="alert" class="renew-error">{{ error }}</p>
    <section class="renew-panel">
      <div class="renew-panel__header">
        <h2>{{ editing ? "编辑规则" : "添加规则" }}</h2>
        <button v-if="editing" class="renew-link" @click="reset">
          取消编辑
        </button>
      </div>
      <form class="renew-rule-form" @submit.prevent="save">
        <label
          >命令名<input
            v-model="comm"
            aria-label="规则命令名"
            :readonly="Boolean(editing)"
            required
            placeholder="例如 curl"
        /></label>
        <label
          >动作<select v-model="action" aria-label="规则动作">
            <option value="ALLOW">允许</option>
            <option value="BLOCK">阻断</option>
            <option value="ALERT">告警</option>
            <option value="REWRITE">重写</option>
          </select></label
        >
        <label
          >优先级<input
            v-model.number="priority"
            type="number"
            aria-label="规则优先级"
        /></label>
        <label
          >正则（可选）<input
            v-model="regex"
            aria-label="规则正则"
            placeholder="匹配命令参数"
        /></label>
        <label v-if="action === 'REWRITE' && regex"
          >替换<input v-model="replacement" aria-label="规则替换"
        /></label>
        <label v-if="action === 'REWRITE' && !regex"
          >重写命令 JSON 数组<input v-model="rewrite" aria-label="重写命令"
        /></label>
        <button
          type="submit"
          class="renew-primary-button"
          :disabled="busy || !ready"
        >
          {{ busy ? "处理中…" : "保存规则" }}
        </button>
      </form>
    </section>
    <section class="renew-panel">
      <div class="renew-table-wrap">
        <table class="renew-table">
          <thead>
            <tr>
              <th>命令</th>
              <th>动作</th>
              <th>优先级</th>
              <th>匹配 / 重写</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(rule, key) in rules" :key="key">
              <td>{{ rule.comm || key }}</td>
              <td>{{ actionLabel(rule.action) }}</td>
              <td>{{ rule.priority || 0 }}</td>
              <td>
                {{ rule.regex || rule.rewritten_cmd?.join(" ") || "全部参数"
                }}<small v-if="rule.replacement">
                  → {{ rule.replacement }}</small
                >
              </td>
              <td>
                <button class="renew-link" :disabled="busy" @click="edit(rule)">
                  编辑
                </button>
                <button
                  class="renew-link"
                  :disabled="busy"
                  @click="pendingDelete = rule.comm || String(key)"
                >
                  删除
                </button>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="ready && !Object.keys(rules).length" class="renew-empty">
          还没有 agent-wrapper 规则
        </p>
      </div>
    </section>
    <section
      v-if="pendingDelete"
      role="alertdialog"
      aria-label="确认删除规则"
      class="renew-panel renew-confirm"
    >
      <p>
        确定删除 {{ pendingDelete }} 的规则？此操作影响所有 Agent 工具的对应命令。
      </p>
      <button class="renew-primary-button" :disabled="busy" @click="remove">
        确认删除
      </button>
      <button class="renew-link" :disabled="busy" @click="pendingDelete = ''">
        取消
      </button>
    </section>
  </div>
</template>
