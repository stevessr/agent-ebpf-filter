<script setup lang="ts">
import { onMounted } from "vue";
import { ThunderboltOutlined } from "@ant-design/icons-vue";
import ConfigMLTab from "../../components/config/ConfigMLTab.vue";
import { useConfigML } from "../../composables/config/useConfigML";

const ml = useConfigML();
const {
  fetchMLStatus,
  fetchAllSamples,
  fetchExistingCommandData,
  fetchResearchSessions,
} = ml;

onMounted(async () => {
  await fetchMLStatus();
  fetchAllSamples();
  fetchExistingCommandData(true);
  fetchResearchSessions(true);
});
</script>

<template>
  <div class="ml-page">
    <div class="ml-page__header">
      <a-typography-title :level="3" class="ml-page__title">
        <ThunderboltOutlined />
        ML Classification
      </a-typography-title>
      <a-typography-paragraph class="ml-page__description">
        本地命令安全模型、参数调优、LLM
        复核和训练集管理现在是独立一级页面，不再嵌套在 Configuration 中。
      </a-typography-paragraph>
    </div>
    <ConfigMLTab :ml="ml" />
  </div>
</template>

<style scoped>
.ml-page {
  min-height: 100%;
  padding: 24px;
  background: #f0f2f5;
}

.ml-page__header {
  margin-bottom: 12px;
}

.ml-page__title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.ml-page__description {
  margin-bottom: 0;
  color: #667085;
}
</style>
