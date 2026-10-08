<script setup lang="ts">
import { computed, provide, reactive, shallowRef, ref, watch } from "vue";
import {
  RouterView,
  loadRouteLocation,
  routeLocationKey,
  routerKey,
  useRouter,
} from "vue-router";
import type {
  RouteLocationRaw,
  RouteLocationNormalizedLoaded,
} from "vue-router";
import { resolvePanelRoute } from "../../composables/workbench/resolvePanelRoute";
const props = defineProps<{ path: string }>();
const emit = defineEmits<{ navigate: [path: string] }>();
const router = useRouter();
const resolved = computed(() => resolvePanelRoute(router, props.path));
const loaded = shallowRef<RouteLocationNormalizedLoaded>();
const loadError = ref("");
watch(
  resolved,
  async (route, _old, onCleanup) => {
    let cancelled = false;
    onCleanup(() => {
      cancelled = true;
    });
    if (loaded.value?.matched.at(-1) !== route.matched.at(-1))
      loaded.value = undefined;
    loadError.value = "";
    try {
      const ready = await loadRouteLocation(route);
      if (!cancelled) loaded.value = ready;
    } catch {
      if (!cancelled) loadError.value = "面板加载失败，请刷新页面重试。";
    }
  },
  { immediate: true },
);
// Each dock owns its route context: changing tabs in one pane must not change another.
const scopedRoute = reactive(
  Object.fromEntries(
    Object.keys(router.currentRoute.value).map((key) => [
      key,
      computed(
        () => resolved.value[key as keyof RouteLocationNormalizedLoaded],
      ),
    ]),
  ),
) as unknown as RouteLocationNormalizedLoaded;
provide(routeLocationKey, scopedRoute);
const navigate = (to: RouteLocationRaw) => {
  emit("navigate", resolvePanelRoute(router, to, resolved.value).fullPath);
  return Promise.resolve();
};
provide(routerKey, {
  ...router,
  currentRoute: resolved,
  resolve: (to: RouteLocationRaw, current?: RouteLocationNormalizedLoaded) =>
    router.resolve(to, current || resolved.value),
  push: navigate,
  replace: navigate,
});
</script>
<template>
  <RouterView v-if="loaded" :route="loaded" /><a-alert
    v-else-if="loadError"
    type="error"
    :message="loadError"
  />
  <div v-else class="pane-loading">正在加载面板…</div>
</template>
