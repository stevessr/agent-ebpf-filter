<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import type { NetworkFlow } from "../../composables/network/useNetworkEnrichment";

interface CaptureProfile {
  id: string;
  vendor: string;
  product?: string;
  operation?: string;
  sources?: string[];
  protocols?: string[];
  directions?: string[];
  methods?: string[];
  host_suffixes?: string[];
  host_contains?: string[];
  path_prefixes?: string[];
  path_contains?: string[];
  required_headers?: string[];
  content_types?: string[];
  transports?: string[];
  families?: string[];
  remote_ports?: number[];
  remote_cidrs?: string[];
  processes?: string[];
  min_score?: number;
}

interface CaptureProfileState {
  path: string;
  customProfiles: CaptureProfile[];
  effectiveProfiles: CaptureProfile[];
  builtinCount: number;
  customCount: number;
  effectiveCount: number;
  updatedAt?: string;
  matcher: {
    generation: number;
    profiles: number;
    dispatchBuckets: number;
    dispatchEntries: number;
    maxBucket: number;
  };
  capabilities: {
    sources: string[];
    protocols: string[];
    directions: string[];
    methods: string[];
    transports: string[];
    families: string[];
    privacyBoundary: string;
    maxPreviewFlows: number;
  };
}

interface PreviewResponse {
  matchedCount: number;
  matches: Array<{ index: number }>;
}

const props = defineProps<{
  flows: NetworkFlow[];
  selectedFlow?: NetworkFlow | null;
}>();

const state = ref<CaptureProfileState | null>(null);
const loading = ref(false);
const saving = ref(false);
const previewing = ref(false);
const editorOpen = ref(false);
const previewIndexes = ref<number[]>([]);
const editingOriginalID = ref("");
const previewLimitReached = computed(
  () => !!state.value && props.flows.length > state.value.capabilities.maxPreviewFlows,
);

const emptyProfile = (): CaptureProfile => ({
  id: "",
  vendor: "custom",
  product: "",
  operation: "",
  sources: ["kernel_socket_prefix", "tls_plaintext"],
  protocols: ["http1"],
  directions: ["outgoing"],
  methods: [],
  host_suffixes: [],
  host_contains: [],
  path_prefixes: [],
  path_contains: [],
  required_headers: [],
  content_types: [],
  transports: [],
  families: [],
  remote_ports: [],
  remote_cidrs: [],
  processes: [],
  min_score: 60,
});

const draft = reactive<CaptureProfile>(emptyProfile());
const remotePortsText = computed({
  get: () => (draft.remote_ports || []).join(", "),
  set: (value: string) => {
    draft.remote_ports = value
      .split(/[\s,]+/)
      .map((item) => Number(item))
      .filter((port) => Number.isInteger(port) && port > 0 && port <= 65535);
  },
});

const resetDraft = (profile?: CaptureProfile) => {
  const next = profile ? JSON.parse(JSON.stringify(profile)) : emptyProfile();
  for (const key of Object.keys(draft) as Array<keyof CaptureProfile>) {
    delete draft[key];
  }
  Object.assign(draft, next);
  previewIndexes.value = [];
};

const loadProfiles = async () => {
  loading.value = true;
  try {
    const response = await axios.get<CaptureProfileState>("/network/capture-profiles");
    state.value = response.data;
  } catch (error: any) {
    message.error(error?.response?.data?.error || error?.message || "Failed to load capture profiles");
  } finally {
    loading.value = false;
  }
};

const normalizedProtocol = (flow: NetworkFlow) => {
  const value = (flow.appProtocol || "").toLowerCase();
  if (value.includes("grpc")) return "grpc";
  if (value.includes("http/2") || value === "http2" || value === "h2") return "http2";
  if (value.includes("http")) return "http1";
  return "http1";
};

const flowHost = (flow: NetworkFlow) =>
  flow.httpHost || flow.sni || flow.dstDomain || flow.dnsName || "";

const safeRuleID = (host: string) => {
  const slug = host
    .toLowerCase()
    .replace(/[^a-z0-9.-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);
  return `custom.${slug || "socket"}`;
};

const draftFromFlow = (flow?: NetworkFlow | null) => {
  if (!flow) {
    message.info("Open a flow detail first, then create a rule from that socket.");
    return;
  }
  const host = flowHost(flow);
  resetDraft({
    ...emptyProfile(),
    id: safeRuleID(host),
    vendor: "custom",
    product: flow.dstService || "",
    operation: flow.httpMethod ? `${flow.httpMethod.toLowerCase()}.request` : "socket.capture",
    protocols: [normalizedProtocol(flow)],
    directions: flow.direction ? [flow.direction.toLowerCase()] : ["outgoing"],
    methods: flow.httpMethod ? [flow.httpMethod.toUpperCase()] : [],
    transports: [(flow.transport || flow.protocol || "tcp").toLowerCase()],
    processes: (flow.processComms || []).slice(0, 1).map((value) => value.toLowerCase()),
    host_suffixes: host ? [host] : [],
    min_score: host ? 70 : 40,
  });
  editingOriginalID.value = "";
  editorOpen.value = true;
  void previewDraft();
};

const createProfile = () => {
  resetDraft();
  editingOriginalID.value = "";
  editorOpen.value = true;
};

const editProfile = (profile: CaptureProfile) => {
  resetDraft(profile);
  editingOriginalID.value = profile.id;
  editorOpen.value = true;
  void previewDraft();
};

const observations = computed(() => {
  const limit = state.value?.capabilities.maxPreviewFlows || 500;
  return props.flows.slice(0, limit).map((flow) => ({
    Source: draft.sources?.[0] || "kernel_socket_prefix",
    Protocol: normalizedProtocol(flow),
    Direction: (flow.direction || "").toLowerCase(),
    Method: (flow.httpMethod || "").toUpperCase(),
    Host: flowHost(flow),
    Path: "",
    Headers: {},
    ContentType: "",
    Transport: (flow.transport || flow.protocol || "").toLowerCase(),
    Family: flow.dstIp ? (flow.dstIp.includes(":") ? "ipv6" : "ipv4") : "",
    RemoteIP: flow.dstIp || "",
    RemotePort: flow.dstPort || 0,
    Process: (flow.processComms?.[0] || "").toLowerCase(),
  }));
});

const previewDraft = async () => {
  if (!draft.id || !draft.vendor) {
    previewIndexes.value = [];
    return;
  }
  previewing.value = true;
  try {
    const response = await axios.post<PreviewResponse>("/network/capture-profiles/preview", {
      profile: draft,
      observations: observations.value,
    });
    previewIndexes.value = response.data.matches.map((item) => item.index);
  } catch (error: any) {
    previewIndexes.value = [];
    if (editorOpen.value) {
      message.warning(error?.response?.data?.error || error?.message || "Profile preview failed");
    }
  } finally {
    previewing.value = false;
  }
};

const matchedFlows = computed(() =>
  previewIndexes.value
    .map((index) => props.flows[index])
    .filter((flow): flow is NetworkFlow => !!flow)
    .slice(0, 8),
);

const saveProfiles = async (profiles: CaptureProfile[]) => {
  saving.value = true;
  try {
    const response = await axios.put<CaptureProfileState>("/network/capture-profiles", { profiles });
    state.value = response.data;
    message.success("Capture profiles published atomically");
    return true;
  } catch (error: any) {
    message.error(error?.response?.data?.error || error?.message || "Failed to save capture profiles");
    return false;
  } finally {
    saving.value = false;
  }
};

const saveDraft = async () => {
  if (!draft.id.trim() || !draft.vendor.trim()) {
    message.warning("Profile ID and vendor are required");
    return;
  }
  const profiles = [...(state.value?.customProfiles || [])];
  const targetID = editingOriginalID.value || draft.id;
  const index = profiles.findIndex((profile) => profile.id === targetID);
  const normalized = JSON.parse(JSON.stringify(draft)) as CaptureProfile;
  if (index >= 0) profiles.splice(index, 1, normalized);
  else profiles.push(normalized);
  if (await saveProfiles(profiles)) {
    editingOriginalID.value = normalized.id;
    editorOpen.value = false;
  }
};

const removeProfile = async (profile: CaptureProfile) => {
  const profiles = (state.value?.customProfiles || []).filter((item) => item.id !== profile.id);
  await saveProfiles(profiles);
};

const profileColumns = [
  { title: "Profile", dataIndex: "id", key: "id", width: 230 },
  { title: "Vendor", dataIndex: "vendor", key: "vendor", width: 120 },
  { title: "Sources", key: "sources", width: 180 },
  { title: "Protocol", key: "protocols", width: 120 },
  { title: "Match", key: "match" },
  { title: "Actions", key: "actions", width: 150 },
];

watch(
  () => props.selectedFlow?.flowId,
  () => {
    if (editorOpen.value && !editingOriginalID.value && props.selectedFlow) {
      draftFromFlow(props.selectedFlow);
    }
  },
);

onMounted(() => void loadProfiles());
</script>

<template>
  <div class="socket-profile-config">
    <a-row :gutter="[12, 12]" style="margin-bottom: 12px">
      <a-col :xs="24" :md="8">
        <a-card size="small" :loading="loading">
          <a-statistic title="Effective profiles" :value="state?.effectiveCount || 0" />
          <div class="meta-line">{{ state?.builtinCount || 0 }} built-in + {{ state?.customCount || 0 }} custom · {{ state?.matcher.dispatchBuckets || 0 }} index buckets · max {{ state?.matcher.maxBucket || 0 }}/bucket</div>
        </a-card>
      </a-col>
      <a-col :xs="24" :md="8">
        <a-card size="small">
          <a-statistic title="Visible sockets/flows" :value="flows.length" />
          <div class="meta-line">Preview uses the current Flow workspace snapshot.</div>
        </a-card>
      </a-col>
      <a-col :xs="24" :md="8">
        <a-card size="small">
          <a-statistic title="Draft matches" :value="previewIndexes.length" />
          <div class="meta-line">Server-side matcher; same scoring semantics as capture.</div>
        </a-card>
      </a-col>
    </a-row>

    <a-alert
      type="info"
      show-icon
      style="margin-bottom: 12px"
      :message="state?.capabilities.privacyBoundary || 'Metadata-only capture boundary'"
      description="Socket-prefix capture is deliberately bounded to HTTP start-lines. Query strings are truncated in eBPF before ringbuf delivery; bodies and credential headers are not inspected by this rule editor."
    />

    <a-card size="small" title="Socket capture profiles" :loading="loading">
      <template #extra>
        <a-space wrap>
          <a-button size="small" @click="loadProfiles">Refresh</a-button>
          <a-button size="small" :disabled="!selectedFlow" @click="draftFromFlow(selectedFlow)">From selected flow</a-button>
          <a-button size="small" type="primary" @click="createProfile">New profile</a-button>
        </a-space>
      </template>

      <a-table
        :columns="profileColumns"
        :data-source="state?.customProfiles || []"
        row-key="id"
        size="small"
        :pagination="{ pageSize: 10, size: 'small' }"
        :locale="{ emptyText: 'No custom profiles. Built-ins remain active.' }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'id'">
            <div style="font-family: monospace; font-size: 12px">{{ record.id }}</div>
            <div class="meta-line">{{ record.product || 'generic' }} / {{ record.operation || 'match' }}</div>
          </template>
          <template v-else-if="column.key === 'sources'">
            <a-space :size="4" wrap>
              <a-tag v-for="source in record.sources || ['any']" :key="source" size="small">{{ source }}</a-tag>
            </a-space>
          </template>
          <template v-else-if="column.key === 'protocols'">
            <a-space :size="4" wrap>
              <a-tag v-for="protocol in record.protocols || ['any']" :key="protocol" color="blue" size="small">{{ protocol }}</a-tag>
            </a-space>
          </template>
          <template v-else-if="column.key === 'match'">
            <div v-if="record.host_suffixes?.length">host: {{ record.host_suffixes.join(', ') }}</div>
            <div v-if="record.path_prefixes?.length">path: {{ record.path_prefixes.join(', ') }}</div>
            <div v-if="record.methods?.length">method: {{ record.methods.join(', ') }}</div>
            <div v-if="record.remote_ports?.length">port: {{ record.remote_ports.join(', ') }}</div>
            <div v-if="record.processes?.length">process: {{ record.processes.join(', ') }}</div>
            <span v-if="!record.host_suffixes?.length && !record.path_prefixes?.length && !record.methods?.length && !record.remote_ports?.length && !record.processes?.length" class="meta-line">score-based generic rule</span>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space>
              <a-button size="small" @click="editProfile(record)">Edit</a-button>
              <a-popconfirm title="Delete this custom profile?" @confirm="removeProfile(record)">
                <a-button size="small" danger>Delete</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
      <div class="profile-path">Managed file: <code>{{ state?.path || '-' }}</code></div>
    </a-card>

    <a-drawer v-model:open="editorOpen" title="Socket capture profile" width="min(720px, 96vw)">
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :xs="24" :md="14">
            <a-form-item label="Profile ID" required>
              <a-input v-model:value="draft.id" placeholder="custom.vendor.operation" @blur="previewDraft" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="10">
            <a-form-item label="Vendor" required>
              <a-input v-model:value="draft.vendor" placeholder="custom" @blur="previewDraft" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :xs="24" :md="12">
            <a-form-item label="Product">
              <a-input v-model:value="draft.product" placeholder="responses" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="12">
            <a-form-item label="Operation">
              <a-input v-model:value="draft.operation" placeholder="responses.create" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-divider orientation="left">Capture source</a-divider>
        <a-row :gutter="12">
          <a-col :xs="24" :md="8">
            <a-form-item label="Sources">
              <a-select v-model:value="draft.sources" mode="multiple" :options="(state?.capabilities.sources || []).map(value => ({ value, label: value }))" @change="previewDraft" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-form-item label="Protocols">
              <a-select v-model:value="draft.protocols" mode="multiple" :options="(state?.capabilities.protocols || []).map(value => ({ value, label: value }))" @change="previewDraft" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="8">
            <a-form-item label="Directions">
              <a-select v-model:value="draft.directions" mode="multiple" :options="(state?.capabilities.directions || []).map(value => ({ value, label: value }))" @change="previewDraft" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="HTTP methods">
          <a-select v-model:value="draft.methods" mode="multiple" :options="(state?.capabilities.methods || []).map(value => ({ value, label: value }))" @change="previewDraft" />
        </a-form-item>

        <a-divider orientation="left">Socket / process scope</a-divider>
        <a-row :gutter="12">
          <a-col :xs="24" :md="12">
            <a-form-item label="Transports">
              <a-select v-model:value="draft.transports" mode="multiple" :options="(state?.capabilities.transports || []).map(value => ({ value, label: value }))" @change="previewDraft" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="12">
            <a-form-item label="Address families">
              <a-select v-model:value="draft.families" mode="multiple" :options="(state?.capabilities.families || []).map(value => ({ value, label: value }))" @change="previewDraft" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="Remote ports">
          <a-input v-model:value="remotePortsText" placeholder="443, 8443" @blur="previewDraft" />
        </a-form-item>
        <a-form-item label="Remote CIDRs">
          <a-select v-model:value="draft.remote_cidrs" mode="tags" placeholder="10.0.0.0/8" @change="previewDraft" />
        </a-form-item>
        <a-form-item label="Processes">
          <a-select v-model:value="draft.processes" mode="tags" placeholder="curl" @change="previewDraft" />
        </a-form-item>

        <a-divider orientation="left">API fingerprint</a-divider>
        <a-form-item label="Host suffixes">
          <a-select v-model:value="draft.host_suffixes" mode="tags" placeholder="api.example.com" @change="previewDraft" />
        </a-form-item>
        <a-form-item label="Host contains">
          <a-select v-model:value="draft.host_contains" mode="tags" placeholder="gateway" @change="previewDraft" />
        </a-form-item>
        <a-form-item label="Path prefixes">
          <a-select v-model:value="draft.path_prefixes" mode="tags" placeholder="/v1/responses" @change="previewDraft" />
        </a-form-item>
        <a-form-item label="Path contains">
          <a-select v-model:value="draft.path_contains" mode="tags" placeholder="/Service/Method" @change="previewDraft" />
        </a-form-item>
        <a-row :gutter="12">
          <a-col :xs="24" :md="12">
            <a-form-item label="Required header names">
              <a-select v-model:value="draft.required_headers" mode="tags" placeholder="content-type" @change="previewDraft" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="12">
            <a-form-item label="Content types">
              <a-select v-model:value="draft.content_types" mode="tags" placeholder="application/grpc" @change="previewDraft" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="Minimum score">
          <a-slider v-model:value="draft.min_score" :min="1" :max="100" :marks="{ 30: 'generic', 60: 'strong', 90: 'strict' }" @change="previewDraft" />
        </a-form-item>

        <a-divider orientation="left">Live flow preview</a-divider>
        <a-space wrap style="margin-bottom: 8px">
          <a-button size="small" :loading="previewing" @click="previewDraft">Re-run preview</a-button>
          <a-tag color="blue">{{ previewIndexes.length }} / {{ Math.min(flows.length, state?.capabilities.maxPreviewFlows || 500) }} match</a-tag>
          <a-tag v-if="previewLimitReached" color="orange">preview capped</a-tag>
        </a-space>
        <a-alert
          v-if="draft.path_prefixes?.length || draft.path_contains?.length"
          type="warning"
          show-icon
          message="Flow preview has no request path"
          description="Path selectors are evaluated on real L7 capture events, but the aggregated socket/flow table only carries host/method metadata. A zero preview here does not invalidate a path-specific profile."
          style="margin-bottom: 8px"
        />
        <a-list size="small" bordered :data-source="matchedFlows" :locale="{ emptyText: 'No visible flows match this draft.' }">
          <template #renderItem="{ item }">
            <a-list-item>
              <a-space wrap>
                <code>{{ item.dstIp }}:{{ item.dstPort }}</code>
                <a-tag v-if="flowHost(item)" size="small">{{ flowHost(item) }}</a-tag>
                <a-tag v-if="item.appProtocol" color="blue" size="small">{{ item.appProtocol }}</a-tag>
                <span class="meta-line">{{ (item.processComms || []).join(', ') || 'unknown process' }}</span>
              </a-space>
            </a-list-item>
          </template>
        </a-list>
      </a-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <a-button @click="editorOpen = false">Cancel</a-button>
          <a-button type="primary" :loading="saving" @click="saveDraft">Publish profile</a-button>
        </div>
      </template>
    </a-drawer>
  </div>
</template>

<style scoped>
.socket-profile-config {
  min-width: 0;
}
.meta-line,
.profile-path {
  color: #64748b;
  font-size: 12px;
}
.profile-path {
  margin-top: 10px;
  overflow-wrap: anywhere;
}
</style>
