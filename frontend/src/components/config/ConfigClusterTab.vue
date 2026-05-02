<script setup lang="ts">
import { ClusterOutlined, ReloadOutlined } from '@ant-design/icons-vue';
import type { useConfigCluster } from '../../composables/useConfigCluster';

const props = defineProps<{
  cluster: ReturnType<typeof useConfigCluster>;
}>();

const {
  clusterState, clusterNodes, selectedClusterTarget,
  fetchClusterNodes,
  applyClusterTarget,
  getClusterRowClass,
  clusterNodeOptions, clusterRoleText, clusterRoleColor,
} = props.cluster;
</script>

<template>
  <a-row :gutter="[24, 24]">
    <a-col :span="24">
      <a-card title="Cluster Status" size="small">
        <template #extra>
          <ClusterOutlined />
        </template>
        <a-row :gutter="[24, 16]">
          <a-col :xs="24" :md="10">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap">
                <span style="font-weight: 600">Mode</span>
                <a-tag :color="clusterRoleColor">{{ clusterRoleText }}</a-tag>
                <a-tag :color="clusterState?.role === 'slave' ? 'orange' : 'green'">
                  {{ clusterState?.role === 'slave' ? 'Managed by master_url' : 'Default master mode' }}
                </a-tag>
              </div>
              <a-descriptions bordered size="small" :column="1">
                <a-descriptions-item label="Node ID">
                  <span class="cluster-value">{{ clusterState?.nodeId || '—' }}</span>
                </a-descriptions-item>
                <a-descriptions-item label="Node Name">
                  <span class="cluster-value">{{ clusterState?.nodeName || '—' }}</span>
                </a-descriptions-item>
                <a-descriptions-item label="Node URL">
                  <span class="cluster-value">{{ clusterState?.nodeUrl || '—' }}</span>
                </a-descriptions-item>
                <a-descriptions-item v-if="clusterState?.role === 'slave'" label="Master URL">
                  <span class="cluster-value">{{ clusterState?.masterUrl || '—' }}</span>
                </a-descriptions-item>
                <a-descriptions-item label="Cluster Auth">
                  <span>
                    {{ clusterState?.accountConfigured ? 'account set' : 'account missing' }}
                    /
                    {{ clusterState?.passwordConfigured ? 'password set' : 'password missing' }}
                  </span>
                </a-descriptions-item>
              </a-descriptions>
            </div>
          </a-col>
          <a-col :xs="24" :md="14">
            <div style="display: flex; flex-direction: column; gap: 12px">
              <a-alert type="info" show-icon
                message="Select the backend you want to inspect. All API/WS traffic is forwarded by the master." />
              <div style="display: flex; gap: 12px; align-items: center; flex-wrap: wrap">
                <span style="font-weight: 600">Active Target</span>
                <a-select v-model:value="selectedClusterTarget" :options="clusterNodeOptions"
                  style="min-width: 280px; flex: 1" :disabled="clusterState?.role === 'slave'"
                  @change="applyClusterTarget" />
                <a-button @click="fetchClusterNodes">
                  <ReloadOutlined /> Refresh Nodes
                </a-button>
              </div>
              <a-table :data-source="clusterNodes" row-key="id" size="small" :pagination="false"
                :row-class-name="getClusterRowClass">
                <a-table-column title="Name" data-index="name" key="name">
                  <template #default="{ text, record }">
                    <span style="font-weight: 600">{{ text }}</span>
                    <a-tag v-if="record.isLocal" color="green" style="margin-left: 8px">local</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Role" data-index="role" key="role">
                  <template #default="{ text }">
                    <a-tag :color="text === 'slave' ? 'orange' : 'green'">{{ text }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Status" data-index="status" key="status">
                  <template #default="{ text }">
                    <a-tag :color="text === 'online' ? 'green' : text === 'stale' ? 'orange' : 'default'">{{ text }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="URL" data-index="url" key="url">
                  <template #default="{ text }">
                    <span class="cluster-url">{{ text }}</span>
                  </template>
                </a-table-column>
                <a-table-column title="Last Seen" data-index="lastSeen" key="lastSeen">
                  <template #default="{ text }">
                    <span>{{ text ? new Date(text).toLocaleString() : '—' }}</span>
                  </template>
                </a-table-column>
                <a-table-column title="Action" key="action" width="120px">
                  <template #default="{ record }">
                    <a-button v-if="!record.isLocal && clusterState?.role === 'master'" type="link"
                      @click="applyClusterTarget(record.id)">
                      Route here
                    </a-button>
                    <span v-else style="color: #999">—</span>
                  </template>
                </a-table-column>
              </a-table>
            </div>
          </a-col>
        </a-row>
      </a-card>
    </a-col>
  </a-row>
</template>

<style scoped>
.cluster-value {
  display: block;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid #dbeafe;
  background: linear-gradient(180deg, #f8fbff 0%, #eef4ff 100%);
  color: #1f2937;
  font-family: var(--mono);
  word-break: break-all;
}
.cluster-url {
  display: inline-block;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  background: #f8fafc;
  color: #111827;
  font-family: var(--mono);
  word-break: break-all;
  white-space: normal;
}
:deep(.cluster-row-active > td) {
  background: #f0f9eb !important;
}
:deep(.cluster-row-local > td) {
  background: #fafcff !important;
}
</style>
