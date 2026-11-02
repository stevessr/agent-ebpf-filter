<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { systemIsDark, watchSystemTheme } from "./composables/useSystemTheme";
import { uiDesignTokens } from "./config/uiDesignTokens";
import zhCN from "ant-design-vue/es/locale/zh_CN";
import enUS from "ant-design-vue/es/locale/en_US";
import {
  webLocale, setWebLocale, translateUI as tr,
  mountLegacyUITranslations, refreshLegacyUITranslations,
  type WebLocale,
} from "./i18n";
import { theme } from "ant-design-vue";
import {
  SearchOutlined,
  MenuOutlined,
  SettingOutlined,
  UndoOutlined,
  ColumnWidthOutlined,
  ColumnHeightOutlined,
} from "@ant-design/icons-vue";
import DockNode from "./components/workbench/DockNode.vue";
import { NAV_GROUPS } from "./config/navigation";
import { useDockWorkbench } from "./composables/workbench/useDockWorkbench";
import { resolveWorkbenchShortcut } from "./composables/workbench/navigationShortcuts";
import { groups } from "./composables/workbench/layout";
import { useRouter, type RouteLocationRaw } from "vue-router";
import "./components/workbench/workbench.css";
const router = useRouter();

let stopSystemTheme: (() => void) | undefined;
let stopLocalization: (() => void) | undefined;
let narrowNavigationMedia: MediaQueryList | undefined;

function onNarrowNavigationChanged(event: MediaQueryListEvent) {
  if (event.matches) sidebar.value = false;
}
const antLocale = computed(() => webLocale.value === "en-US" ? enUS : zhCN);
const localeChoice = computed({
  get: () => webLocale.value,
  set: (value: WebLocale) => setWebLocale(value),
});
watch(webLocale, async () => { await nextTick(); refreshLegacyUITranslations(); });
onMounted(() => {
  stopSystemTheme = watchSystemTheme();
  stopLocalization = mountLegacyUITranslations();
  narrowNavigationMedia = window.matchMedia("(max-width: 720px)");
  if (narrowNavigationMedia.matches) sidebar.value = false;
  narrowNavigationMedia.addEventListener("change", onNarrowNavigationChanged);
  window.addEventListener("keydown", onWorkbenchKeydown);
});
onUnmounted(() => {
  stopSystemTheme?.();
  stopLocalization?.();
  narrowNavigationMedia?.removeEventListener("change", onNarrowNavigationChanged);
  window.removeEventListener("keydown", onWorkbenchKeydown);
});

const antTheme = computed(() => ({
  algorithm: systemIsDark.value ? theme.darkAlgorithm : theme.defaultAlgorithm,
  token: {
    colorPrimary: systemIsDark.value ? uiDesignTokens.dark.accent : uiDesignTokens.light.accent,
    colorBgBase: systemIsDark.value ? uiDesignTokens.dark.canvas : uiDesignTokens.light.canvas,
    colorBgContainer: systemIsDark.value ? uiDesignTokens.dark.surface : uiDesignTokens.light.surface,
    colorBorder: systemIsDark.value ? uiDesignTokens.dark.border : uiDesignTokens.light.border,
    colorText: systemIsDark.value ? uiDesignTokens.dark.text : uiDesignTokens.light.text,
    borderRadius: uiDesignTokens.radius,
    fontSize: uiDesignTokens.fontSize,
  },
}));
const {
  notice,
  storageAvailable,
  root,
  focused,
  dragging,
  open,
  focus,
  select,
  close,
  drag,
  drop,
  split,
  resize,
  navigate,
  reset,
} = useDockWorkbench();
const sidebar = ref(true),
  searchInput = ref<HTMLInputElement | null>(null),
  navigationToggle = ref<HTMLButtonElement | null>(null),
  query = ref(""),
  activeGroup = ref("observability");
const navGroups = computed(() =>
  NAV_GROUPS.map((g) => ({
    ...g,
    children: g.children.filter((c) =>
      (c.title.toLowerCase().includes(query.value.toLowerCase()) ||
        tr(c.title).toLowerCase().includes(query.value.toLowerCase())),
    ),
  })).filter((g) => g.children.length),
);
const paneCount = computed(() => groups(root.value).length);
const activePane = computed(() =>
  groups(root.value).find((g) => g.id === focused.value),
);
const activeTitle = computed(
  () =>
    activePane.value?.tabs.find((t) => t.id === activePane.value?.active)
      ?.title || "工作空间",
);
function onWorkbenchKeydown(event: KeyboardEvent) {
  const target = event.target;
  const inside = (selector: string) =>
    target instanceof Element && !!target.closest(selector);
  const action = resolveWorkbenchShortcut({
    key: event.key,
    ctrlKey: event.ctrlKey,
    metaKey: event.metaKey,
    defaultPrevented: event.defaultPrevented,
    inEditorOrTerminal: inside(".monaco-editor, .xterm, .wterm"),
    inEditableField: inside('input, textarea, select, [contenteditable="true"]'),
    inSidebarSearch: target === searchInput.value,
    narrow: !!narrowNavigationMedia?.matches,
    sidebarOpen: sidebar.value,
  });
  if (!action) return;
  event.preventDefault();
  if (action === "close-overlay") {
    sidebar.value = false;
    void nextTick(() => navigationToggle.value?.focus());
    return;
  }
  sidebar.value = true;
  searchInput.value?.focus();
  searchInput.value?.select();
}
function openNavigationItem(route: RouteLocationRaw) {
  open(router.resolve(route).fullPath);
  if (narrowNavigationMedia?.matches) sidebar.value = false;
  query.value = "";
}
function activity(key: string) {
  if (activeGroup.value === key) sidebar.value = !sidebar.value;
  else {
    activeGroup.value = key;
    sidebar.value = true;
  }
}
function resetLayout() {
  if (window.confirm(tr("恢复单面板布局？现有分屏标签将关闭，后端配置不会改变。")))
    reset();
}
</script>
<template>
  <a-config-provider
    :locale="antLocale"
    :theme="antTheme"
  >
    <div class="ide-workbench">
      <header class="ide-titlebar">
        <strong>Agent eBPF</strong
        ><span class="ide-title-divider">/</span
        ><span class="ide-title-caption">Observability studio</span>
        <label class="ide-command"
          ><SearchOutlined /><input
            ref="searchInput"
            type="search"
            aria-keyshortcuts="Control+K Meta+K"
            v-model="query"
            aria-label="搜索工作台功能"
            :placeholder="tr('搜索功能 / 工作空间')"
          /><kbd>Ctrl / Cmd + K</kbd></label
        >
        <label class="ide-locale-picker" for="webui-language">{{ tr('语言') }} 
          <select id="webui-language" v-model="localeChoice" data-testid="locale-switch" aria-label="Language / 语言">
            <option value="zh-CN">简体中文</option>
            <option value="en-US">English</option>
          </select>
        </label>
        <button
          ref="navigationToggle"
          title="显示 / 隐藏导航"
          aria-label="显示 / 隐藏导航"
          aria-controls="ide-sidebar"
          :aria-expanded="sidebar"
          @click="sidebar = !sidebar"
        >
          <MenuOutlined />
        </button>
        <button title="重置布局" aria-label="重置布局" @click="resetLayout">
          <UndoOutlined />
        </button>
      </header>
      <div class="ide-body">
        <button v-if="sidebar" class="ide-sidebar-scrim" type="button" :aria-label="tr('关闭导航')" @click="sidebar = false" />
        <nav class="ide-activity" aria-label="活动栏">
          <button
            v-for="g in NAV_GROUPS"
            :key="g.key"
            :title="g.title"
            :aria-label="g.title"
            :class="{ selected: activeGroup === g.key && sidebar }"
            :aria-pressed="activeGroup === g.key && sidebar"
            @click="activity(g.key)"
          >
            <component :is="g.icon" />
          </button>
          <div class="ide-activity-spacer" />
          <button
            title="系统配置"
            aria-label="系统配置"
            @click="openNavigationItem('/config/registry')"
          >
            <SettingOutlined />
          </button>
        </nav>
        <aside v-show="sidebar" id="ide-sidebar" class="ide-sidebar">
          <div class="ide-sidebar-heading">
            {{ tr("工作空间") }} <span>{{ webLocale === "en-US" ? "Local" : "本地" }}</span>
          </div>
          <div class="ide-project">
            <span class="ide-project-dot" />agent-ebpf-filter
            <span class="ide-project-badge">Linux</span>
          </div>
          <div class="ide-nav-scroll">
            <details
              v-for="g in navGroups"
              :key="g.key"
              :open="!!query || activeGroup === g.key"
            >
              <summary>
                {{ tr(g.title) }}<span>{{ g.children.length }}</span>
              </summary>
              <button
                v-for="item in g.children"
                :key="item.key"
                class="ide-nav-item"
                :class="{ selected: activeTitle === item.title }"
                draggable="true"
                :title="tr(item.title) + ' · ' + tr('可拖拽到工作台分屏')"
                @click="openNavigationItem(item.defaultRoute)"
                @dragstart="drag('nav:' + item.key, $event)"
                @dragend="dragging = ''"
              >
                <component :is="item.icon" /><span>{{ tr(item.title) }}</span
                >
              </button>
            </details>
            <p v-if="!navGroups.length" class="ide-nav-empty">{{ tr("没有匹配的功能") }}</p>
          </div>
          <div class="ide-sidebar-hint">
            <span>{{ tr("自定义工作台") }}</span>
            <p>{{ tr("拖动功能或标签到面板边缘分屏，中央合并。") }}</p>
            <div>
              <button
                aria-label="左右分屏"
                @click="split(focused, 'horizontal')"
              >
                <ColumnWidthOutlined /> {{ tr("左右分屏") }}</button
              ><button
                aria-label="上下分屏"
                @click="split(focused, 'vertical')"
              >
                <ColumnHeightOutlined /> {{ tr("上下分屏") }}
              </button>
            </div>
          </div>
        </aside>
        <main class="ide-editor">
          <DockNode
            :node="root"
            :focused="focused"
            :dragging="!!dragging"
            @focus="focus"
            @select="select"
            @close="close"
            @drag="drag"
            @end="dragging = ''"
            @drop="drop"
            @split="split"
            @resize="resize"
            @navigate="navigate"
          />
        </main>
      </div>
      <footer class="ide-statusbar">
        <span class="ide-status-remote">{{ webLocale === "en-US" ? "Local" : "本地" }}</span><span>{{ tr("eBPF 工作台") }}</span
        ><span>{{ webLocale === "en-US" ? paneCount + " panes" : paneCount + " 个面板" }}</span
        ><span class="ide-status-spacer" /><span>{{ tr(activeTitle) }}</span>
        ><span v-if="notice" class="ide-layout-notice" role="status">{{
          notice
        }}</span>
        <span>{{ tr(storageAvailable ? "布局自动保存" : "布局仅本次有效") }}</span
        ><button @click="resetLayout">{{ tr("重置布局") }}</button>
      </footer>
    </div>
  </a-config-provider>
</template>
