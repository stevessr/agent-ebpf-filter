import { ref, computed } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import {
  getStoredClusterTarget,
  isLocalClusterTarget,
  normalizeClusterTarget,
  setStoredClusterTarget,
} from '../utils/requestContext';
import type { ClusterNodeInfo, ClusterStateResponse } from '../types/config';

export function useConfigCluster() {
  const clusterState = ref<ClusterStateResponse | null>(null);
  const clusterNodes = ref<ClusterNodeInfo[]>([]);
  const selectedClusterTarget = ref(getStoredClusterTarget());

  const fetchClusterState = async () => {
    try {
      const res = await axios.get('/cluster/state');
      clusterState.value = res.data as ClusterStateResponse;
    } catch (_) {
      console.error('Failed to fetch cluster state');
    }
  };

  const fetchClusterNodes = async () => {
    try {
      const res = await axios.get('/cluster/nodes');
      clusterNodes.value = (res.data?.nodes || []) as ClusterNodeInfo[];
      if (
        !clusterNodes.value.some(
          (node) => node.id === selectedClusterTarget.value,
        ) &&
        !isLocalClusterTarget(selectedClusterTarget.value)
      ) {
        setStoredClusterTarget('local');
        selectedClusterTarget.value = 'local';
      }
    } catch (_) {
      console.error('Failed to fetch cluster nodes');
    }
  };

  const updateClusterTargetFromStorage = () => {
    selectedClusterTarget.value = getStoredClusterTarget();
  };

  const applyClusterTarget = (target: string) => {
    const normalized = normalizeClusterTarget(target);
    setStoredClusterTarget(normalized);
    selectedClusterTarget.value = normalized;
    message.success(
      normalized === 'local'
        ? 'Routed back to local master'
        : 'Cluster target updated',
    );
    window.location.reload();
  };

  const getClusterRowClass = (record: ClusterNodeInfo) => {
    if (record.id === selectedClusterTarget.value) return 'cluster-row-active';
    if (record.isLocal) return 'cluster-row-local';
    return '';
  };

  const clusterNodeOptions = computed(() => [
    { label: 'Local master', value: 'local' },
    ...clusterNodes.value
      .filter((node) => !node.isLocal)
      .map((node) => ({
        label: `${node.name} · ${node.status}`,
        value: node.id,
      })),
  ]);

  const clusterRoleText = computed(() => {
    if (!clusterState.value) return 'Unknown';
    return clusterState.value.role === 'master' ? 'Master' : 'Slave';
  });

  const clusterRoleColor = computed(() =>
    clusterState.value?.role === 'slave' ? 'orange' : 'green',
  );

  return {
    clusterState, clusterNodes, selectedClusterTarget,
    fetchClusterState, fetchClusterNodes,
    updateClusterTargetFromStorage, applyClusterTarget,
    getClusterRowClass,
    clusterNodeOptions, clusterRoleText, clusterRoleColor,
  };
}
