<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  mode: "NONE" | "COUNTER" | "BLOCKLIST";
  keyField: "uid" | "pid" | "comm";
  limit: number;
}>();

const emit = defineEmits<{
  (e: "update:mode", val: "NONE" | "COUNTER" | "BLOCKLIST"): void;
  (e: "update:keyField", val: "uid" | "pid" | "comm"): void;
  (e: "update:limit", val: number): void;
}>();

const localMode = computed({
  get: () => props.mode,
  set: (val) => emit("update:mode", val),
});

const localKeyField = computed({
  get: () => props.keyField,
  set: (val) => emit("update:keyField", val),
});

const localLimit = computed({
  get: () => props.limit,
  set: (val) => emit("update:limit", val),
});
</script>

<template>
  <div class="block-card block-map" style="border: 1px solid #2f54eb; margin-bottom: 10px; box-shadow: 0 4px 10px rgba(47, 84, 235, 0.05);">
    <div class="block-header" style="background: #2f54eb;">
      <span class="block-badge" style="background: rgba(0, 0, 0, 0.25)">Block 2.5</span>
      <strong style="color: #fff">低代码 Map 状态化存储积木 (Map Stateful Operations)</strong>
    </div>
    <div class="block-body">
      <div class="desc-line" style="font-size: 13px; color: #595959; margin-bottom: 12px;">
        选择是否启用 BPF 内核高性能 Map Stateful 数据流运算进行状态化追踪判定：
      </div>
      <a-row :gutter="12">
        <a-col :span="8">
          <div style="font-size: 11px; color: #8c8c8c; margin-bottom: 4px;">Map 运行模式</div>
          <a-select v-model:value="localMode" style="width: 100%">
            <a-select-option value="NONE">无状态 (直接决策)</a-select-option>
            <a-select-option value="COUNTER">计数器限频 (COUNTER)</a-select-option>
            <a-select-option value="BLOCKLIST">外部 Hash 黑名单判定 (BLOCKLIST)</a-select-option>
          </a-select>
        </a-col>
        <a-col :span="8" v-if="localMode !== 'NONE'">
          <div style="font-size: 11px; color: #8c8c8c; margin-bottom: 4px;">操作追踪主键 (Map Key)</div>
          <a-select v-model:value="localKeyField" style="width: 100%">
            <a-select-option value="pid">当前进程 PID</a-select-option>
            <a-select-option value="uid">当前用户 UID</a-select-option>
            <a-select-option value="comm">当前进程名 (Comm)</a-select-option>
          </a-select>
        </a-col>
        <a-col :span="8" v-if="localMode === 'COUNTER'">
          <div style="font-size: 11px; color: #8c8c8c; margin-bottom: 4px;">阈值限制 (Max Threshold)</div>
          <a-input-number v-model:value="localLimit" :min="1" :max="10000" style="width: 100%" />
        </a-col>
      </a-row>
      <div v-if="localMode !== 'NONE'" class="helper-text" style="color: #2f54eb; margin-top: 10px; font-size: 11px;">
        * 状态机制将自动在内核声明 eBPF HASH 映射表。满足以上累计命中过滤规则的阈值条件后，才下发执行 Block 3 终极动作。
      </div>
    </div>
  </div>
</template>
