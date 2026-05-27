import { ref, computed, watch, onMounted } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';

export type LaunchEnvEntry = {
  id: string;
  key: string;
  value: string;
  enabled: boolean;
};

export type LaunchEnvProfile = {
  id: string;
  name: string;
  entries: LaunchEnvEntry[];
};

export type DetectedLaunchEnvEntry = {
  key: string;
  value: string;
};

const LAUNCH_ENV_STORAGE_KEY = 'executor_launch_env_v2';
const LAUNCH_ENV_LEGACY_KEY = 'executor_launch_env';

function loadProfiles(): LaunchEnvProfile[] {
  try {
    const raw = JSON.parse(localStorage.getItem(LAUNCH_ENV_STORAGE_KEY) || 'null') as unknown;
    if (Array.isArray(raw) && raw.length > 0) {
      return raw as LaunchEnvProfile[];
    }
    const legacy = JSON.parse(localStorage.getItem(LAUNCH_ENV_LEGACY_KEY) || '[]') as LaunchEnvEntry[];
    if (Array.isArray(legacy) && legacy.length > 0) {
      return [{ id: 'default', name: 'Default Profile', entries: legacy }];
    }
    return [{ id: 'default', name: 'Default Profile', entries: [] }];
  } catch {
    return [{ id: 'default', name: 'Default Profile', entries: [] }];
  }
}

export function useLaunchEnv() {
  const profiles = ref<LaunchEnvProfile[]>(loadProfiles());
  const activeProfileId = ref<string>(localStorage.getItem('executor_active_profile_id') || profiles.value[0]?.id || '');
  const activeProfile = computed(() => profiles.value.find(p => p.id === activeProfileId.value) || profiles.value[0]);
  const launchEnvEntries = computed({
    get: () => activeProfile.value?.entries || [],
    set: (val) => {
      const p = profiles.value.find(p => p.id === activeProfileId.value);
      if (p) p.entries = val;
    },
  });

  const newLaunchEnvKey = ref('');
  const newLaunchEnvValue = ref('');
  const detectedLaunchEnvEntries = ref<DetectedLaunchEnvEntry[]>([]);
  const detectedLaunchEnvSearch = ref('');
  const detectedLaunchEnvLoading = ref(false);
  const detectedLaunchEnvError = ref('');

  const profileRenameModalOpen = ref(false);
  const profileRenameValue = ref('');
  const profileRenameId = ref('');

  function persistProfiles() {
    localStorage.setItem(LAUNCH_ENV_STORAGE_KEY, JSON.stringify(profiles.value));
    localStorage.setItem('executor_active_profile_id', activeProfileId.value);
  }

  function addNewProfile() {
    const id = `profile-${Date.now()}`;
    profiles.value.push({ id, name: `New Profile ${profiles.value.length + 1}`, entries: [] });
    activeProfileId.value = id;
  }

  function copyProfile(profile: LaunchEnvProfile) {
    const id = `profile-${Date.now()}`;
    profiles.value.push({ id, name: `${profile.name} (Copy)`, entries: JSON.parse(JSON.stringify(profile.entries)) as LaunchEnvEntry[] });
    activeProfileId.value = id;
  }

  function deleteProfile(id: string) {
    if (profiles.value.length <= 1) { message.warning('Cannot delete the last profile'); return; }
    const index = profiles.value.findIndex(p => p.id === id);
    if (index >= 0) {
      profiles.value.splice(index, 1);
      if (activeProfileId.value === id) activeProfileId.value = profiles.value[0].id;
    }
  }

  function openRenameModal(profile: LaunchEnvProfile) {
    profileRenameId.value = profile.id;
    profileRenameValue.value = profile.name;
    profileRenameModalOpen.value = true;
  }

  function applyRename() {
    const p = profiles.value.find(p => p.id === profileRenameId.value);
    if (p && profileRenameValue.value.trim()) p.name = profileRenameValue.value.trim();
    profileRenameModalOpen.value = false;
  }

  const isValidLaunchEnvKey = (key: string) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(key);

  const launchEnvEntriesCount = computed(() =>
    launchEnvEntries.value.filter(e => e.enabled && isValidLaunchEnvKey(e.key.trim())).length
  );

  const launchEnvEntryKeys = computed(() =>
    new Set(launchEnvEntries.value.map(e => e.key.trim()).filter(k => Boolean(k)))
  );

  const launchEnvRecord = computed<Record<string, string>>(() => {
    const env: Record<string, string> = {};
    for (const entry of launchEnvEntries.value) {
      const key = entry.key.trim();
      if (!entry.enabled || !key || !isValidLaunchEnvKey(key)) continue;
      env[key] = entry.value;
    }
    return env;
  });

  const launchEnvPreview = computed(() => {
    const entries = Object.entries(launchEnvRecord.value);
    return entries.length ? entries.map(([k, v]) => `${k}=${v || '""'}`).join('  ') : 'No launch environment overrides configured';
  });

  const launchEnvScope = computed(() => {
    const count = launchEnvEntriesCount.value;
    return count ? `${count} active variable${count === 1 ? '' : 's'} applied to Remote, Shell, Tmux, and Script launches` : 'Applies to all Executor launches';
  });

  const launchEnvColumns = [
    { title: 'Enabled', key: 'enabled', dataIndex: 'enabled' },
    { title: 'Key', key: 'key', dataIndex: 'key' },
    { title: 'Value', key: 'value', dataIndex: 'value' },
    { title: 'Action', key: 'action', dataIndex: 'action' },
  ];

  const detectedLaunchEnvColumns = [
    { title: 'Key', key: 'key', dataIndex: 'key' },
    { title: 'Value', key: 'value', dataIndex: 'value' },
    { title: 'Action', key: 'action', dataIndex: 'action' },
  ];

  const filteredDetectedLaunchEnvEntries = computed(() => {
    const query = detectedLaunchEnvSearch.value.trim().toLowerCase();
    if (!query) return detectedLaunchEnvEntries.value;
    return detectedLaunchEnvEntries.value.filter(e => e.key.toLowerCase().includes(query) || e.value.toLowerCase().includes(query));
  });

  watch([profiles, activeProfileId], persistProfiles, { deep: true });

  const addLaunchEnvEntry = () => {
    const key = newLaunchEnvKey.value.trim();
    if (!key) { message.error('Please enter an environment variable name'); return; }
    if (!isValidLaunchEnvKey(key)) { message.error('Environment variable names should look like FOO or FOO_BAR'); return; }
    launchEnvEntries.value = [{ id: `launch-env-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`, key, value: newLaunchEnvValue.value, enabled: true }, ...launchEnvEntries.value];
    newLaunchEnvKey.value = '';
    newLaunchEnvValue.value = '';
  };

  const removeLaunchEnvEntry = (id: string) => {
    launchEnvEntries.value = launchEnvEntries.value.filter(e => e.id !== id);
  };

  const clearDisabledLaunchEnvEntries = () => {
    launchEnvEntries.value = launchEnvEntries.value.filter(e => e.enabled);
  };

  const isLaunchEnvImported = (key: string) => launchEnvEntryKeys.value.has(key.trim());

  const importDetectedLaunchEnvEntry = (entry: DetectedLaunchEnvEntry) => {
    const key = entry.key.trim();
    if (!key || !isValidLaunchEnvKey(key)) return;
    const next: LaunchEnvEntry = { id: `launch-env-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`, key, value: entry.value, enabled: true };
    const index = launchEnvEntries.value.findIndex(item => item.key.trim() === key);
    if (index >= 0) {
      const current = launchEnvEntries.value[index];
      launchEnvEntries.value = [...launchEnvEntries.value.slice(0, index), { ...current, key, value: entry.value, enabled: true }, ...launchEnvEntries.value.slice(index + 1)];
      return;
    }
    launchEnvEntries.value = [next, ...launchEnvEntries.value];
  };

  const importAllDetectedLaunchEnvEntries = () => {
    for (const entry of filteredDetectedLaunchEnvEntries.value) importDetectedLaunchEnvEntry(entry);
  };

  const refreshDetectedLaunchEnvEntries = async () => {
    detectedLaunchEnvLoading.value = true;
    detectedLaunchEnvError.value = '';
    try {
      const res = await axios.get('/system/env');
      const items = Array.isArray(res.data?.items) ? (res.data.items as DetectedLaunchEnvEntry[]) : [];
      detectedLaunchEnvEntries.value = items.map(item => ({ key: String(item?.key || '').trim(), value: String(item?.value ?? '') })).filter(item => Boolean(item.key));
    } catch (err: any) {
      detectedLaunchEnvError.value = err?.response?.data?.error || err?.message || 'Failed to load detected env vars';
    } finally {
      detectedLaunchEnvLoading.value = false;
    }
  };

  onMounted(() => { void refreshDetectedLaunchEnvEntries(); });

  return {
    profiles, activeProfileId, activeProfile,
    newLaunchEnvKey, newLaunchEnvValue,
    launchEnvEntries, launchEnvEntriesCount, launchEnvColumns,
    launchEnvPreview, launchEnvScope, launchEnvRecord,
    addLaunchEnvEntry, removeLaunchEnvEntry, clearDisabledLaunchEnvEntries,
    addNewProfile, copyProfile, deleteProfile, openRenameModal,
    detectedLaunchEnvEntries, detectedLaunchEnvSearch, detectedLaunchEnvLoading, detectedLaunchEnvError,
    filteredDetectedLaunchEnvEntries, detectedLaunchEnvColumns,
    refreshDetectedLaunchEnvEntries, importAllDetectedLaunchEnvEntries,
    isLaunchEnvImported, importDetectedLaunchEnvEntry,
    profileRenameModalOpen, profileRenameValue, applyRename,
  };
}
