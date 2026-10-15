<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { systemIsDark, watchSystemTheme } from "./composables/useSystemTheme";
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
import { groups } from "./composables/workbench/layout";
import { useRouter } from "vue-router";
import "./components/workbench/workbench.css";
const router = useRouter();

let stopSystemTheme: (() => void) | undefined;
let stopLocalization: (() => void) | undefined;
const antLocale = computed(() => webLocale.value === "en-US" ? enUS : zhCN);
const localeChoice = computed({
  get: () => webLocale.value,
  set: (value: WebLocale) => setWebLocale(value),
});
watch(webLocale, async () => { await nextTick(); refreshLegacyUITranslations(); });
onMounted(() => {
  stopSystemTheme = watchSystemTheme();
  stopLocalization = mountLegacyUITranslations();
});
onUnmounted(() => { stopSystemTheme?.(); stopLocalization?.(); });

const antTheme = computed(() => ({
  algorithm: systemIsDark.value ? theme.darkAlgorithm : theme.defaultAlgorithm,
  token: {
    colorPrimary: systemIsDark.value ? "#7b9fff" : "#2563eb",
    colorBgBase: systemIsDark.value ? "#111318" : "#f8fafc",
    colorBgContainer: systemIsDark.value ? "#191c22" : "#ffffff",
    colorBorder: systemIsDark.value ? "#2b303b" : "#d9e2ef",
    borderRadius: 4,
    fontSize: 13,
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
function activity(key: string) {
  if (activeGroup.value === key) sidebar.value = !sidebar.value;
  else {
    activeGroup.value = key;
    sidebar.value = true;
  }
}
function resetLayout() {
  if (window.confirm("恢复单面板布局？现有分屏标签将关闭，后端配置不会改变。"))
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
        <span class="ide-logo">◈</span><strong>Agent eBPF</strong
        ><span class="ide-title-divider">/</span
        ><span class="ide-title-caption">Observability Studio</span>
        <label class="ide-command"
          ><SearchOutlined /><input
            v-model="query"
            aria-label="搜索工作台功能"
            :placeholder="tr('搜索功能 / 工作空间')"
          /><kbd>IDE</kbd></label
        >
        <label class="ide-locale-picker" for="webui-language">${tr('语言')} 
          <select id="webui-language" v-model="localeChoice" data-testid="locale-switch" aria-label="Language / 语言">
            <option value="zh-CN">简体中文</option>
            <option value="en-US">English</option>
          </select>
        </label>
        <button
          title="显示 / 隐藏导航"
          aria-label="显示 / 隐藏导航"
          @click="sidebar = !sidebar"
        >
          <MenuOutlined />
        </button>
        <button title="重置布局" aria-label="重置布局" @click="resetLayout">
          <UndoOutlined />
        </button>
      </header>
      <div class="ide-body">
        <nav class="ide-activity" aria-label="活动栏">
          <button
            v-for="g in NAV_GROUPS"
            :key="g.key"
            :title="g.title"
            :aria-label="g.title"
            :class="{ selected: activeGroup === g.key && sidebar }"
            @click="activity(g.key)"
          >
            <component :is="g.icon" />
          </button>
          <div class="ide-activity-spacer" />
          <button
            title="系统配置"
            aria-label="系统配置"
            @click="open('/config/registry')"
          >
            <SettingOutlined />
          </button>
        </nav>
        <aside v-if="sidebar" class="ide-sidebar">
          <div class="ide-sidebar-heading">
            {{ tr("工作空间") }} <span>AGENT / LOCAL</span>
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
                @click="open(router.resolve(item.defaultRoute).fullPath)"
                @dragstart="drag('nav:' + item.key, $event)"
                @dragend="dragging = ''"
              >
                <component :is="item.icon" /><span>{{ tr(item.title) }}</span
                ><span class="ide-nav-grip">⠿</span>
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
        <span class="ide-status-remote">⌁ LOCAL</span><span>◈ {{ tr("eBPF 工作台") }}</span
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
