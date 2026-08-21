<script setup lang="ts">
import type { useConfigML } from "../../../composables/config/useConfigML";
import MLModelTuneSection from "./ModelTab/MLModelTuneSection.vue";
import MLModelTrainingSection from "./ModelTab/MLModelTrainingSection.vue";

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();
</script>

<template>
  <MLModelTuneSection :ml="props.ml" />
  <MLModelTrainingSection :ml="props.ml" />
</template>
