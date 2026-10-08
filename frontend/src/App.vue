<script setup lang="ts">
import { computed, ref } from "vue";
import zhCN from "ant-design-vue/es/locale/zh_CN";
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
      c.title.toLowerCase().includes(query.value.toLowerCase()),
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
    :locale="zhCN"
    :theme="{
      algorithm: theme.darkAlgorithm,
      token: {
        colorPrimary: '#7b9fff',
        colorBgBase: '#111318',
        colorBgContainer: '#191c22',
        colorBorder: '#2b303b',
        borderRadius: 4,
        fontSize: 13,
      },
    }"
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
            placeholder="搜索功能 / 工作空间"
          /><kbd>IDE</kbd></label
        >
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
            工作空间 <span>AGENT / LOCAL</span>
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
                {{ g.title }}<span>{{ g.children.length }}</span>
              </summary>
              <button
                v-for="item in g.children"
                :key="item.key"
                class="ide-nav-item"
                :class="{ selected: activeTitle === item.title }"
                draggable="true"
                :title="item.title + ' · 可拖拽到工作台分屏'"
                @click="open(router.resolve(item.defaultRoute).fullPath)"
                @dragstart="drag('nav:' + item.key, $event)"
                @dragend="dragging = ''"
              >
                <component :is="item.icon" /><span>{{ item.title }}</span
                ><span class="ide-nav-grip">⠿</span>
              </button>
            </details>
            <p v-if="!navGroups.length" class="ide-nav-empty">没有匹配的功能</p>
          </div>
          <div class="ide-sidebar-hint">
            <span>自定义工作台</span>
            <p>拖动功能或标签到面板边缘分屏，中央合并。</p>
            <div>
              <button
                aria-label="左右分屏"
                @click="split(focused, 'horizontal')"
              >
                <ColumnWidthOutlined /> 左右分屏</button
              ><button
                aria-label="上下分屏"
                @click="split(focused, 'vertical')"
              >
                <ColumnHeightOutlined /> 上下分屏
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
        <span class="ide-status-remote">⌁ LOCAL</span><span>◈ eBPF 工作台</span
        ><span>{{ paneCount }} 个面板</span
        ><span class="ide-status-spacer" /><span>{{ activeTitle }}</span
        ><span v-if="notice" class="ide-layout-notice" role="status">{{
          notice
        }}</span>
        <span>{{ storageAvailable ? "布局自动保存" : "布局仅本次有效" }}</span
        ><button @click="resetLayout">重置布局</button>
      </footer>
    </div>
  </a-config-provider>
</template>
