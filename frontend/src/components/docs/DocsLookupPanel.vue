<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { message } from "ant-design-vue";
import {
  BookOutlined,
  CopyOutlined,
  LinkOutlined,
  SearchOutlined,
} from "@ant-design/icons-vue";
import DocPreviewPane from "./DocPreviewPane.vue";
import {
  linuxReferenceCatalog,
  linuxReferenceQuickQueries,
  linuxReferenceRelease,
  linuxReferenceScopes,
  type LinuxReferenceEntry,
  type LinuxReferenceKind,
} from "../../data/linuxReferenceCatalog";

type SearchScope = "all" | LinuxReferenceKind;

const searchText = ref("");
const searchScope = ref<SearchScope>("all");
const releaseLabel = linuxReferenceRelease;

const featuredNames = new Set([
  "openat",
  "execve",
  "bpf",
  "bpf_map_lookup_elem",
  "bpf_probe_read_user_str",
  "seccomp",
  "setns",
  "fanotify_mark",
]);

const normalizedQuery = computed(() => searchText.value.trim().toLowerCase());

const matchesScope = (entry: LinuxReferenceEntry) =>
  searchScope.value === "all" || entry.kind === searchScope.value;

const scoreEntry = (entry: LinuxReferenceEntry, query: string) => {
  if (!query) return featuredNames.has(entry.name) ? 10 : 0;

  const haystack = [
    entry.name,
    ...(entry.aliases || []),
    entry.category,
    entry.summary,
    entry.synopsis,
    ...(entry.keywords || []),
  ]
    .join(" ")
    .toLowerCase();

  let score = 0;
  if (entry.name.toLowerCase() === query) score += 1000;
  if ((entry.aliases || []).some((alias) => alias.toLowerCase() === query)) score += 900;
  if (entry.name.toLowerCase().startsWith(query)) score += 700;
  if ((entry.aliases || []).some((alias) => alias.toLowerCase().startsWith(query))) score += 650;
  if (haystack.includes(query)) score += 300;

  for (const token of query.split(/[\s,]+/).filter(Boolean)) {
    if (entry.name.toLowerCase().includes(token)) score += 100;
    else if (haystack.includes(token)) score += 30;
  }

  if (featuredNames.has(entry.name)) score += 25;
  return score;
};

const sortedEntries = computed(() => {
  const query = normalizedQuery.value;
  const candidates = linuxReferenceCatalog.filter(matchesScope);

  return candidates
    .map((entry) => ({
      entry,
      score: scoreEntry(entry, query),
    }))
    .filter(({ score }) => score > 0 || !query)
    .sort((a, b) => {
      if (query) {
        if (b.score !== a.score) return b.score - a.score;
      } else {
        const featuredDelta =
          Number(featuredNames.has(b.entry.name)) - Number(featuredNames.has(a.entry.name));
        if (featuredDelta !== 0) return featuredDelta;
      }

      if (a.entry.kind !== b.entry.kind) {
        return a.entry.kind === "syscall" ? -1 : 1;
      }
      if (a.entry.category !== b.entry.category) {
        return a.entry.category.localeCompare(b.entry.category);
      }
      return a.entry.name.localeCompare(b.entry.name);
    })
    .map(({ entry }) => entry);
});

const selectedEntryId = ref("");

watch(
  sortedEntries,
  (entries) => {
    if (entries.length === 0) {
      selectedEntryId.value = "";
      return;
    }

    if (!entries.some((entry) => entry.id === selectedEntryId.value)) {
      selectedEntryId.value = entries[0].id;
    }
  },
  { immediate: true },
);

const selectedEntry = computed(() => {
  if (!sortedEntries.value.length) return null;
  return (
    sortedEntries.value.find((entry) => entry.id === selectedEntryId.value) ||
    sortedEntries.value[0] ||
    null
  );
});

const totalSyscalls = computed(() =>
  linuxReferenceCatalog.filter((entry) => entry.kind === "syscall").length,
);

const totalHelpers = computed(() =>
  linuxReferenceCatalog.filter((entry) => entry.kind === "helper").length,
);

const copyText = async (text: string) => {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      message.success("Link copied");
      return;
    }

    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.readOnly = true;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    document.body.removeChild(textarea);
    message.success("Link copied");
  } catch (err) {
    message.error("Failed to copy link");
  }
};

const openDocs = (url: string) => {
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
};

const applyQuickQuery = (query: string) => {
  searchText.value = query;
};

const clearSearch = () => {
  searchText.value = "";
  searchScope.value = "all";
};

const selectEntry = (entry: LinuxReferenceEntry) => {
  selectedEntryId.value = entry.id;
};

const getRowClassName = (record: LinuxReferenceEntry) =>
  record.id === selectedEntryId.value ? "docs-row--selected" : "";

const getRowClickHandlers = (record: LinuxReferenceEntry) => ({
  onClick: () => selectEntry(record),
});
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-card title="Quick Reference Search" size="small">
        <template #extra>
          <a-tag color="gold">{{ releaseLabel }}</a-tag>
          <a-tag color="blue" style="margin-left: 8px;">
            <BookOutlined /> {{ totalSyscalls }} syscalls
          </a-tag>
          <a-tag color="green" style="margin-left: 8px;">{{ totalHelpers }} helpers</a-tag>
        </template>

        <a-alert
          type="info"
          show-icon
          style="margin-bottom: 16px;"
          :message="`Search by syscall name, helper name, alias, or keyword. Pick a row below to preview the local snapshot on the right. Cached files live under /linux-docs/6.18.`"
        />

        <a-row :gutter="[12, 12]" align="middle">
          <a-col :xs="24" :md="14">
            <a-input-search
              v-model:value="searchText"
              allow-clear
              placeholder="Try openat, execve, bpf_map_lookup_elem, bpf_probe_read_user_str..."
              size="large"
              @search="applyQuickQuery"
            >
              <template #prefix><SearchOutlined /></template>
            </a-input-search>
          </a-col>
          <a-col :xs="24" :md="6">
            <a-select
              v-model:value="searchScope"
              :options="linuxReferenceScopes"
              size="large"
              style="width: 100%"
            />
          </a-col>
          <a-col :xs="24" :md="4" style="text-align: right;">
            <a-button @click="clearSearch">Reset</a-button>
          </a-col>
        </a-row>

        <div style="margin-top: 16px;">
          <div style="margin-bottom: 8px; color: #8c8c8c; font-size: 12px;">
            Quick picks
          </div>
          <a-space wrap>
            <a-button
              v-for="q in linuxReferenceQuickQueries"
              :key="q"
              size="small"
              :type="searchText === q ? 'primary' : 'default'"
              @click="applyQuickQuery(q)"
            >
              {{ q }}
            </a-button>
          </a-space>
        </div>
      </a-card>
    </a-col>

    <a-col :xs="24" :xl="10">
      <a-card class="docs-index-card" title="Local Snapshot Index" size="small">
        <template #extra>
          <a-tag color="purple">{{ sortedEntries.length }} matches</a-tag>
        </template>

        <a-empty
          v-if="sortedEntries.length === 0"
          description="No cached snapshot matched the current filter. Try another quick pick or clear the filter."
          style="padding: 40px 0;"
        />

        <a-table
          v-else
          :data-source="sortedEntries"
          row-key="id"
          :pagination="{ pageSize: 8, showSizeChanger: false }"
          :customRow="getRowClickHandlers"
          :rowClassName="getRowClassName"
          size="small"
          :table-layout="'fixed'"
        >
          <a-table-column title="Reference" key="reference">
            <template #default="{ record }">
              <div style="display: flex; flex-direction: column; gap: 4px;">
                <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
                  <a-typography-text strong>{{ record.name }}</a-typography-text>
                  <a-tag :color="record.kind === 'syscall' ? 'blue' : 'green'">
                    {{ record.kind === "syscall" ? "syscall" : "eBPF helper" }}
                  </a-tag>
                  <a-tag color="purple">{{ record.category }}</a-tag>
                </div>
                <div v-if="record.aliases?.length" style="color: #888; font-size: 12px;">
                  Aliases: <code>{{ record.aliases.join(", ") }}</code>
                </div>
              </div>
            </template>
          </a-table-column>

          <a-table-column title="Summary" key="summary">
            <template #default="{ record }">
              <div style="max-width: 100%; color: #444;">
                {{ record.summary }}
              </div>
            </template>
          </a-table-column>

          <a-table-column title="Synopsis" key="synopsis">
            <template #default="{ record }">
              <code style="white-space: normal;">{{ record.synopsis }}</code>
            </template>
          </a-table-column>

          <a-table-column title="Actions" key="actions" width="180px">
            <template #default="{ record }">
              <div style="display: flex; gap: 8px; flex-wrap: wrap;">
                <a-button type="link" size="small" @click="selectEntry(record)">
                  Preview
                </a-button>
                <a-button type="link" size="small" @click="openDocs(record.url)">
                  <LinkOutlined /> Source
                </a-button>
                <a-button type="link" size="small" @click="copyText(record.url)">
                  <CopyOutlined /> Copy
                </a-button>
              </div>
            </template>
          </a-table-column>
        </a-table>
      </a-card>
    </a-col>

    <a-col :xs="24" :xl="14">
      <DocPreviewPane :entry="selectedEntry" />
    </a-col>
  </a-row>
</template>

<style scoped>
.docs-index-card :deep(.ant-table-row:hover td) {
  cursor: pointer;
}

.docs-index-card :deep(.docs-row--selected td) {
  background: #e6f4ff !important;
}
</style>
