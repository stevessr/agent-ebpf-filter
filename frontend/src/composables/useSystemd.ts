import { ref, computed } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';

export interface SystemdService {
  unit: string; load: string; active: string; sub: string; description: string;
}

export function useSystemd() {
  const systemdServices = ref<SystemdService[]>([]);
  const systemdLoading = ref(false);
  const systemdSearch = ref('');
  const systemdScope = ref<'system' | 'user'>('system');

  const showLogsModal = ref(false);
  const activeLogUnit = ref('');
  const serviceLogs = ref('');
  const logsLoading = ref(false);

  const filteredSystemdServices = computed(() => {
    if (!systemdSearch.value.trim()) return systemdServices.value;
    const q = systemdSearch.value.toLowerCase();
    return systemdServices.value.filter(s => s.unit.toLowerCase().includes(q) || s.description.toLowerCase().includes(q));
  });

  const systemdColumns = [
    { title: 'Unit', dataIndex: 'unit', key: 'unit', sorter: (a: any, b: any) => a.unit.localeCompare(b.unit) },
    { title: 'Active', dataIndex: 'active', key: 'active', width: 120, filters: [{ text: 'active', value: 'active' }, { text: 'inactive', value: 'inactive' }], onFilter: (v: string, r: any) => r.active === v },
    { title: 'Sub', dataIndex: 'sub', key: 'sub', width: 140 },
    { title: 'Description', dataIndex: 'description', key: 'description', ellipsis: true },
    { title: 'Action', key: 'action', width: 220, align: 'right' },
  ];

  const fetchSystemdServices = async () => {
    systemdLoading.value = true;
    try {
      const res = await axios.get(`/system/systemd?scope=${systemdScope.value}`);
      systemdServices.value = res.data;
    } catch (err) { message.error(`Failed to fetch ${systemdScope.value} systemd services`); } finally { systemdLoading.value = false; }
  };

  const controlSystemdService = async (unit: string, action: string) => {
    try {
      await axios.post('/system/systemd/control', { unit, action, scope: systemdScope.value });
      message.success(`${systemdScope.value.toUpperCase()} service ${unit} ${action} command sent`);
      void fetchSystemdServices();
    } catch (err: any) { message.error(err?.response?.data?.error || `Failed to ${action} service`); }
  };

  const fetchSystemdLogs = async (unit: string) => {
    activeLogUnit.value = unit; showLogsModal.value = true; logsLoading.value = true; serviceLogs.value = '';
    try {
      const res = await axios.get(`/system/systemd/logs?unit=${unit}&lines=200&scope=${systemdScope.value}`);
      serviceLogs.value = res.data.logs;
    } catch (err) { message.error('Failed to fetch logs'); } finally { logsLoading.value = false; }
  };

  return {
    systemdServices, systemdLoading, systemdSearch, systemdScope,
    showLogsModal, activeLogUnit, serviceLogs, logsLoading,
    filteredSystemdServices, systemdColumns,
    fetchSystemdServices, controlSystemdService, fetchSystemdLogs,
  };
}
