<script setup lang="ts">
import { ref, watch } from 'vue';
import { useMLStatusStream } from '../../composables/useMLStatusStream';
import type { useConfigML } from '../../composables/useConfigML';
import ConfigMLStatusTab from './ml/ConfigMLStatusTab.vue';
import ConfigMLParamsTab from './ml/ConfigMLParamsTab.vue';
import ConfigMLModelTab from './ml/ConfigMLModelTab.vue';
import ConfigMLLLMTab from './ml/ConfigMLLLMTab.vue';
import ConfigMLTrainingTab from './ml/ConfigMLTrainingTab.vue';

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const { wsActive, applyMLStatusResponse } = props.ml;

// WebSocket status stream
const { connect: wsConnect } = useMLStatusStream(applyMLStatusResponse);

const mlSubTabKey = ref(localStorage.getItem('config_ml_subtab') || 'status');

watch(mlSubTabKey, (val) => {
  localStorage.setItem('config_ml_subtab', val);
});

wsActive.value = true;
wsConnect();
</script>

<template>
  <a-tabs
    v-model:activeKey="mlSubTabKey"
    size="small"
    type="card"
    style="margin: 8px 0 16px"
  >
    <a-tab-pane key="status" tab="状况"></a-tab-pane>
    <a-tab-pane key="params" tab="参数"></a-tab-pane>
    <a-tab-pane key="model" tab="模型管理"></a-tab-pane>
    <a-tab-pane key="llm" tab="LLM 打分"></a-tab-pane>
    <a-tab-pane key="training" tab="训练集管理"></a-tab-pane>
  </a-tabs>

  <a-row :gutter="[24, 24]">
    <template v-if="mlSubTabKey === 'status'">
      <ConfigMLStatusTab :ml="props.ml" @nav="mlSubTabKey = $event" />
    </template>
    <template v-if="mlSubTabKey === 'params'">
      <ConfigMLParamsTab :ml="props.ml" @nav="mlSubTabKey = $event" />
    </template>
    <template v-if="mlSubTabKey === 'model'">
      <ConfigMLModelTab :ml="props.ml" />
    </template>
    <template v-if="mlSubTabKey === 'llm'">
      <ConfigMLLLMTab :ml="props.ml" />
    </template>
    <template v-if="mlSubTabKey === 'training'">
      <ConfigMLTrainingTab :ml="props.ml" />
    </template>
  </a-row>
</template>
