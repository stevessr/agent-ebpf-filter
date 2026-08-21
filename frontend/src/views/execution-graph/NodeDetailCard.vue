<script setup lang="ts">
import {
  AlertOutlined,
  InfoCircleOutlined,
  SafetyCertificateOutlined,
  StopOutlined,
} from "@ant-design/icons-vue";
import type {
  ExecutionDetailTab,
  ExecutionGraphEdge,
  ExecutionGraphNode,
} from "../../types/executionGraph";

const props = defineProps<{
  selectedNode: ExecutionGraphNode | null;
  activeDetailTab: ExecutionDetailTab;
  selectedNodeKindColor: string;
  actionableComm: string;
  metadataEntries: [string, unknown][];
  relatedProcesses: ExecutionGraphNode[];
  processTreeNodes: ExecutionGraphNode[];
  relatedFiles: ExecutionGraphNode[];
  relatedNetwork: ExecutionGraphNode[];
  relatedPolicies: ExecutionGraphNode[];
  incidentEdges: ExecutionGraphEdge[];
}>();

const emit = defineEmits<{
  "update:activeDetailTab": [tab: string];
  selectNode: [id: string];
  addRule: [action: "ALLOW" | "BLOCK"];
  exportSample: [label: "ALLOW" | "ALERT" | "BLOCK"];
  focusTab: [tab: ExecutionDetailTab];
}>();

const renderNodeSubtitle = (node: ExecutionGraphNode) =>
  node.subtitle?.trim() ||
  node.metadata?.path ||
  node.metadata?.endpoint ||
  "—";
</script>

<template>
  <a-card :bordered="false" class="detail-card">
    <template #title
      ><span><InfoCircleOutlined /> Node Details</span></template
    >
    <template #extra>
      <a-space v-if="selectedNode">
        <a-tag :color="selectedNodeKindColor">{{ selectedNode.kind }}</a-tag>
        <a-tag v-if="selectedNode.riskScore !== undefined" color="volcano"
          >risk {{ Number(selectedNode.riskScore).toFixed(0) }}</a-tag
        >
      </a-space>
    </template>
    <a-empty
      v-if="!selectedNode"
      description="Select a node from the graph to inspect context, resources, and actions."
    />
    <template v-else>
      <a-space direction="vertical" size="middle" style="width: 100%">
        <div>
          <a-typography-title :level="5" style="margin-bottom: 6px">{{
            selectedNode.label
          }}</a-typography-title>
          <a-typography-paragraph type="secondary" style="margin-bottom: 0">
            {{ renderNodeSubtitle(selectedNode) }}
          </a-typography-paragraph>
        </div>
        <a-descriptions :column="1" size="small" bordered>
          <a-descriptions-item label="Node ID">{{
            selectedNode.id
          }}</a-descriptions-item>
          <a-descriptions-item label="Kind">{{
            selectedNode.kind
          }}</a-descriptions-item>
          <a-descriptions-item v-if="selectedNode.pid" label="PID">{{
            selectedNode.pid
          }}</a-descriptions-item>
          <a-descriptions-item
            v-if="actionableComm"
            label="Actionable Command"
            >{{ actionableComm }}</a-descriptions-item
          >
        </a-descriptions>
        <div class="node-actions">
          <a-space wrap>
            <a-button size="small" @click="emit('addRule', 'ALLOW')"
              ><SafetyCertificateOutlined /> Add allow rule</a-button
            >
            <a-button size="small" danger @click="emit('addRule', 'BLOCK')"
              ><StopOutlined /> Add block rule</a-button
            >
            <a-button size="small" @click="emit('exportSample', 'ALLOW')"
              >Mark benign</a-button
            >
            <a-button
              size="small"
              type="primary"
              ghost
              @click="emit('exportSample', 'ALERT')"
              ><AlertOutlined /> Mark suspicious</a-button
            >
            <a-button
              size="small"
              type="dashed"
              @click="emit('exportSample', 'BLOCK')"
              >Export BLOCK sample</a-button
            >
          </a-space>
        </div>
        <a-space wrap>
          <a-button size="small" @click="emit('focusTab', 'processes')"
            >Show related process tree</a-button
          >
          <a-button size="small" @click="emit('focusTab', 'files')"
            >Show related files</a-button
          >
          <a-button size="small" @click="emit('focusTab', 'network')"
            >Show related network flows</a-button
          >
          <a-button size="small" @click="emit('focusTab', 'policy')"
            >Show related policy events</a-button
          >
        </a-space>
        <a-tabs
          :activeKey="activeDetailTab"
          size="small"
          @update:activeKey="emit('update:activeDetailTab', String($event))"
        >
          <a-tab-pane
            key="processes"
            :tab="`Processes (${relatedProcesses.length})`"
          >
            <a-list
              size="small"
              :data-source="
                selectedNode?.kind === 'process'
                  ? processTreeNodes
                  : relatedProcesses
              "
              bordered
            >
              <template #renderItem="{ item }">
                <a-list-item
                  @click="emit('selectNode', item.id)"
                  class="clickable-list-item"
                >
                  <a-space direction="vertical" size="small">
                    <span
                      ><b>{{ item.label }}</b>
                      <a-tag color="green">process</a-tag></span
                    >
                    <span class="muted-line">{{
                      renderNodeSubtitle(item)
                    }}</span>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>
          <a-tab-pane key="files" :tab="`Files (${relatedFiles.length})`">
            <a-list size="small" :data-source="relatedFiles" bordered>
              <template #renderItem="{ item }">
                <a-list-item
                  @click="emit('selectNode', item.id)"
                  class="clickable-list-item"
                >
                  <a-space direction="vertical" size="small">
                    <span
                      ><b>{{ item.label }}</b></span
                    >
                    <span class="muted-line">{{
                      item.metadata?.path || "file access"
                    }}</span>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>
          <a-tab-pane key="network" :tab="`Network (${relatedNetwork.length})`">
            <a-list size="small" :data-source="relatedNetwork" bordered>
              <template #renderItem="{ item }">
                <a-list-item
                  @click="emit('selectNode', item.id)"
                  class="clickable-list-item"
                >
                  <a-space direction="vertical" size="small">
                    <span
                      ><b>{{ item.label }}</b></span
                    >
                    <span class="muted-line">{{
                      item.subtitle ||
                      item.metadata?.domain ||
                      "network relation"
                    }}</span>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>
          <a-tab-pane key="policy" :tab="`Policy (${relatedPolicies.length})`">
            <a-list size="small" :data-source="relatedPolicies" bordered>
              <template #renderItem="{ item }">
                <a-list-item
                  @click="emit('selectNode', item.id)"
                  class="clickable-list-item"
                >
                  <a-space direction="vertical" size="small">
                    <span>
                      <b>{{ item.label }}</b>
                      <a-tag
                        :color="
                          item.kind === 'policy_alert' ? 'error' : 'default'
                        "
                        >{{ item.kind }}</a-tag
                      >
                    </span>
                    <span class="muted-line">{{
                      renderNodeSubtitle(item)
                    }}</span>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>
          <a-tab-pane key="edges" :tab="`Edges (${incidentEdges.length})`">
            <a-list size="small" :data-source="incidentEdges" bordered>
              <template #renderItem="{ item }">
                <a-list-item>
                  <a-space direction="vertical" size="small">
                    <span
                      ><b>{{ item.kind }}</b></span
                    >
                    <span class="muted-line"
                      >{{ item.source }} → {{ item.target }}</span
                    >
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>
          <a-tab-pane key="metadata" tab="Metadata">
            <a-list size="small" :data-source="metadataEntries" bordered>
              <template #renderItem="{ item }">
                <a-list-item>
                  <div class="metadata-row">
                    <span class="metadata-key">{{ item[0] }}</span>
                    <span class="metadata-value">{{ item[1] || "—" }}</span>
                  </div>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>
        </a-tabs>
      </a-space>
    </template>
  </a-card>
</template>
