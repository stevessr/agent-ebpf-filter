<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { FRONTEND_BUILD_FEATURE_MODE } from "../../config/featureFlags";

const route = useRoute();
const router = useRouter();

const feature = computed(() => String(route.query.feature || "unknown"));
const from = computed(() => String(route.query.from || ""));

const goToRuntimeConfig = () => {
  void router.push({ name: "Config", params: { tab: "runtime" } });
};

const goToDashboard = () => {
  void router.push({ name: "Dashboard" });
};
</script>

<template>
  <div class="feature-unavailable">
    <a-card title="此构建未包含该功能" class="feature-unavailable__card">
      <a-alert
        type="warning"
        show-icon
        message="当前前端构建未声明此功能可用。"
        :description="`feature=${feature}，build mode=${FRONTEND_BUILD_FEATURE_MODE}${from ? `，from=${from}` : ''}`"
      />
      <a-typography-paragraph>
        如果后端二进制也未编译该功能，对应 API 会返回 compiled-out
        状态或不会注册路由。若只是运行时关闭，请在 Runtime Config 中启用相应
        gate；若构建时裁剪了功能，需要使用
        <code>AGENT_BUILD_FEATURES</code> /
        <code>AGENT_FRONTEND_BUILD_FEATURES</code>
        重新构建。
      </a-typography-paragraph>
      <div class="feature-unavailable__actions">
        <a-button type="primary" @click="goToRuntimeConfig"
          >查看 Runtime Config</a-button
        >
        <a-button @click="goToDashboard">返回 Dashboard</a-button>
      </div>
    </a-card>
  </div>
</template>

<style scoped>
.feature-unavailable {
  display: flex;
  justify-content: center;
  padding: 64px 16px;
}

.feature-unavailable__card {
  width: min(720px, 100%);
}

.feature-unavailable__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 16px;
}
</style>
