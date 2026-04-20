<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import axios from 'axios';
import { SettingOutlined, DeleteOutlined, PlusOutlined, FilterOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

interface AgentEvent {
  key: string;
  pid: number;
  type: string;
  tag: string;
  comm: string;
  path: string;
  time: string;
}

interface TrackedItem {
  comm: string;
  tag: string;
}

const events = ref<AgentEvent[]>([]);
const isConnected = ref(false);
const showSettings = ref(false);
const trackedItems = ref<TrackedItem[]>([]);
const newCommName = ref('');
const newCommTag = ref('AI Agent');
const selectedTag = ref<string | null>(null);
let ws: WebSocket | null = null;

const tags = ['AI Agent', 'Git', 'Build Tool', 'Package Manager', 'Runtime', 'System Tool', 'Network Tool'];

const fetchTrackedComms = async () => {
  try {
    const res = await axios.get('/config/comms');
    trackedItems.value = res.data;
  } catch (err) {
    console.error('Failed to fetch tracked comms', err);
  }
};

const addComm = async () => {
  if (!newCommName.value) return;
  try {
    await axios.post('/config/comms', { 
      comm: newCommName.value,
      tag: newCommTag.value
    });
    message.success(`Added ${newCommName.value} to tracked commands`);
    newCommName.value = '';
    fetchTrackedComms();
  } catch (err) {
    message.error('Failed to add tracked command');
  }
};

const removeComm = async (comm: string) => {
  try {
    await axios.delete(`/config/comms/${comm}`);
    message.success(`Removed ${comm}`);
    fetchTrackedComms();
  } catch (err) {
    message.error('Failed to remove tracked command');
  }
};

const filteredEvents = computed(() => {
  if (!selectedTag.value) return events.value;
  return events.value.filter(e => e.tag === selectedTag.value);
});

const groupedTrackedItems = computed(() => {
  const groups: Record<string, string[]> = {};
  trackedItems.value.forEach(item => {
    if (!groups[item.tag]) groups[item.tag] = [];
    groups[item.tag].push(item.comm);
  });
  return groups;
});

const columns = [
  {
    title: 'Time',
    dataIndex: 'time',
    key: 'time',
    width: 120,
  },
  {
    title: 'Tag',
    dataIndex: 'tag',
    key: 'tag',
    width: 120,
  },
  {
    title: 'PID',
    dataIndex: 'pid',
    key: 'pid',
    width: 100,
  },
  {
    title: 'Command',
    dataIndex: 'comm',
    key: 'comm',
    width: 150,
  },
  {
    title: 'Event Type',
    dataIndex: 'type',
    key: 'type',
    width: 150,
  },
  {
    title: 'Path',
    dataIndex: 'path',
    key: 'path',
    ellipsis: true,
  },
];

const getTagColor = (type: string) => {
  const colors: Record<string, string> = {
    'execve': 'blue',
    'openat': 'green',
    'network_connect': 'orange',
    'network_bind': 'volcano',
    'mkdir': 'cyan',
    'unlink': 'red',
    'ioctl': 'purple',
  };
  return colors[type] || 'default';
};

const getCategoryColor = (tag: string) => {
  const colors: Record<string, string> = {
    'AI Agent': 'magenta',
    'Git': 'orange',
    'Build Tool': 'cyan',
    'Package Manager': 'green',
    'Runtime': 'blue',
    'System Tool': 'geekblue',
    'Network Tool': 'purple',
  };
  return colors[tag] || 'default';
};

const connectWebSocket = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  ws = new WebSocket(`${protocol}//${host}/ws`);

  ws.onopen = () => {
    isConnected.value = true;
    console.log('Connected to backend eBPF stream');
  };

  ws.onmessage = (message) => {
    try {
      const data = JSON.parse(message.data);
      const now = new Date();
      events.value.unshift({
        key: `${data.pid}-${data.path}-${Date.now()}-${Math.random()}`,
        pid: data.pid,
        type: data.type,
        tag: data.tag,
        comm: data.comm,
        path: data.path,
        time: now.toLocaleTimeString(),
      });
      
      if (events.value.length > 1000) {
        events.value.pop();
      }
    } catch (e) {
      console.error('Failed to parse message', e);
    }
  };

  ws.onclose = () => {
    isConnected.value = false;
    setTimeout(connectWebSocket, 3000);
  };
  
  ws.onerror = (error) => {
    console.error('WebSocket error:', error);
    ws?.close();
  };
};

const clearEvents = () => {
  events.value = [];
};

onMounted(() => {
  connectWebSocket();
  fetchTrackedComms();
});

onUnmounted(() => {
  if (ws) {
    ws.close();
  }
});
</script>

<template>
  <a-layout class="layout">
    <a-layout-header class="header">
      <div class="logo">
        <a-typography-title :level="3" style="color: white; margin: 0; line-height: 64px;">
          Agent eBPF Tracker
        </a-typography-title>
      </div>
      <div style="flex: 1"></div>
      <a-button type="primary" shape="circle" @click="showSettings = true" style="margin-left: auto;">
        <template #icon><SettingOutlined /></template>
      </a-button>
    </a-layout-header>
    <a-layout-content style="padding: 0 50px; margin-top: 24px;">
      <div style="background: #fff; padding: 24px; min-height: 280px">
        <div style="display: flex; justify-content: space-between; margin-bottom: 16px; align-items: center;">
          <div style="display: flex; align-items: center; gap: 16px;">
            <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
            <span>Total Events: {{ events.length }}</span>
            <a-divider type="vertical" />
            <a-select v-model:value="selectedTag" placeholder="Filter by Tag" style="width: 160px" allowClear>
              <template #suffixIcon><FilterOutlined /></template>
              <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
            </a-select>
          </div>
          <a-button type="primary" danger @click="clearEvents">Clear Events</a-button>
        </div>
        <a-table 
          :dataSource="filteredEvents" 
          :columns="columns" 
          size="small"
          :pagination="{ pageSize: 20 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'type'">
              <a-tag :color="getTagColor(record.type)">
                {{ record.type.toUpperCase() }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'tag'">
              <a-tag :color="getCategoryColor(record.tag)">
                {{ record.tag }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'path'">
              <a-typography-text code>{{ record.path }}</a-typography-text>
            </template>
          </template>
        </a-table>
      </div>
    </a-layout-content>

    <a-drawer
      title="Global Filters & Tagging"
      placement="right"
      :closable="true"
      :open="showSettings"
      @close="showSettings = false"
      width="450"
    >
      <div style="margin-bottom: 24px; background: #fafafa; padding: 16px; border-radius: 8px;">
        <h4 style="margin-top: 0">Add New Filter</h4>
        <div style="display: flex; flex-direction: column; gap: 8px;">
          <a-input v-model:value="newCommName" placeholder="Executable name (e.g. gcc)" @pressEnter="addComm" />
          <div style="display: flex; gap: 8px;">
            <a-select v-model:value="newCommTag" style="flex: 1">
              <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
            </a-select>
            <a-button type="primary" @click="addComm">
              <template #icon><PlusOutlined /></template>
              Add
            </a-button>
          </div>
        </div>
      </div>
      
      <div v-for="(comms, tag) in groupedTrackedItems" :key="tag" style="margin-bottom: 16px;">
        <a-divider orientation="left" style="margin: 8px 0">
          <a-tag :color="getCategoryColor(tag as string)">{{ tag }}</a-tag>
        </a-divider>
        <div style="display: flex; flex-wrap: wrap; gap: 8px; padding-left: 8px;">
          <a-tag 
            v-for="comm in comms" 
            :key="comm" 
            closable 
            @close.prevent="removeComm(comm)"
            style="margin-bottom: 4px;"
          >
            {{ comm }}
          </a-tag>
        </div>
      </div>
    </a-drawer>
  </a-layout>
</template>

<style scoped>
.layout {
  min-height: 100vh;
}
.header {
  display: flex;
  align-items: center;
}
</style>
