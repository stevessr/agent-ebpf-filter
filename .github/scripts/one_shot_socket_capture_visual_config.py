from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing socket-config anchor in {path}: {old[:160]!r}")
    write(path, text.replace(old, new, 1))


# ---------------------------------------------------------------------------
# Backend control plane: one durable overlay file shared by the watcher and UI.
# ---------------------------------------------------------------------------
write("backend/app/captureprofile_control.go", r'''package app

import (
    "agent-ebpf-filter/app/captureprofile"
    "agent-ebpf-filter/app/platform"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
)

const captureProfileOverlayFilename = "api-capture-profiles.json"
const maxCaptureProfilePreviewObservations = 500

type captureProfileCapabilities struct {
    Sources          []string `json:"sources"`
    Protocols        []string `json:"protocols"`
    Directions       []string `json:"directions"`
    Methods          []string `json:"methods"`
    PrivacyBoundary  string   `json:"privacyBoundary"`
    MaxPreviewFlows  int      `json:"maxPreviewFlows"`
}

type captureProfileStateResponse struct {
    Path              string                     `json:"path"`
    CustomProfiles    []captureprofile.Profile   `json:"customProfiles"`
    EffectiveProfiles []captureprofile.Profile   `json:"effectiveProfiles"`
    BuiltinCount      int                        `json:"builtinCount"`
    CustomCount       int                        `json:"customCount"`
    EffectiveCount    int                        `json:"effectiveCount"`
    UpdatedAt         string                     `json:"updatedAt,omitempty"`
    Capabilities      captureProfileCapabilities `json:"capabilities"`
}

type captureProfileUpdateRequest struct {
    Profiles []captureprofile.Profile `json:"profiles"`
}

type captureProfilePreviewRequest struct {
    Profile      captureprofile.Profile       `json:"profile"`
    Observations []captureprofile.Observation `json:"observations"`
}

type captureProfilePreviewMatch struct {
    Index int                  `json:"index"`
    Match captureprofile.Match `json:"match"`
}

type captureProfilePreviewResponse struct {
    Profile      captureprofile.Profile        `json:"profile"`
    MatchedCount int                           `json:"matchedCount"`
    Matches      []captureProfilePreviewMatch  `json:"matches"`
}

func captureProfileOverlayPath() string {
    if configured := strings.TrimSpace(os.Getenv("AGENT_EBPF_API_PROFILES")); configured != "" {
        return filepath.Clean(configured)
    }
    return filepath.Join(platform.RuntimeSettingsDir(), captureProfileOverlayFilename)
}

func captureProfileCapabilitiesValue() captureProfileCapabilities {
    return captureProfileCapabilities{
        Sources:         []string{"kernel_socket_prefix", "tls_plaintext"},
        Protocols:       []string{"http1", "http2", "grpc"},
        Directions:      []string{"outgoing", "incoming"},
        Methods:         []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT"},
        PrivacyBoundary: "metadata-only: request/response start-line, host/path/method and header names; no body, credentials or query secrets",
        MaxPreviewFlows: maxCaptureProfilePreviewObservations,
    }
}

func validateCaptureProfileOverlay(profiles []captureprofile.Profile) ([]captureprofile.Profile, error) {
    registry := captureprofile.NewRegistry(nil)
    if err := registry.Replace(profiles); err != nil {
        return nil, err
    }
    return registry.Profiles(), nil
}

func ensureCaptureProfileOverlayFile(path string) error {
    if strings.TrimSpace(path) == "" {
        return errors.New("API capture profile path is empty")
    }
    if _, err := os.Stat(path); err == nil {
        return nil
    } else if !os.IsNotExist(err) {
        return fmt.Errorf("stat API capture profile file: %w", err)
    }
    if err := platform.MkdirAllAsRealUser(filepath.Dir(path), 0755); err != nil {
        return fmt.Errorf("create API capture profile directory: %w", err)
    }
    if err := platform.WriteFileAsRealUser(path, []byte("[]\n"), 0644); err != nil {
        return fmt.Errorf("initialize API capture profile file: %w", err)
    }
    return nil
}

func readCaptureProfileOverlay(path string) ([]captureprofile.Profile, error) {
    if err := ensureCaptureProfileOverlayFile(path); err != nil {
        return nil, err
    }
    return captureprofile.LoadJSON(path)
}

func persistCaptureProfileOverlay(path string, profiles []captureprofile.Profile) error {
    normalized, err := validateCaptureProfileOverlay(profiles)
    if err != nil {
        return err
    }
    payload, err := json.MarshalIndent(normalized, "", "  ")
    if err != nil {
        return fmt.Errorf("encode API capture profiles: %w", err)
    }
    payload = append(payload, '\n')
    if err := platform.MkdirAllAsRealUser(filepath.Dir(path), 0755); err != nil {
        return fmt.Errorf("create API capture profile directory: %w", err)
    }
    tmp := path + ".tmp"
    if err := platform.WriteFileAsRealUser(tmp, payload, 0644); err != nil {
        return fmt.Errorf("write API capture profile staging file: %w", err)
    }
    if err := os.Rename(tmp, path); err != nil {
        _ = os.Remove(tmp)
        return fmt.Errorf("publish API capture profiles atomically: %w", err)
    }
    if err := captureprofile.ReloadDefaultJSON(path); err != nil {
        return fmt.Errorf("publish API capture profile registry: %w", err)
    }
    return nil
}

func buildCaptureProfileState(path string) (captureProfileStateResponse, error) {
    custom, err := readCaptureProfileOverlay(path)
    if err != nil {
        return captureProfileStateResponse{}, err
    }
    updatedAt := ""
    if info, err := os.Stat(path); err == nil {
        updatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
    }
    builtin := captureprofile.BuiltinProfiles()
    effective := captureprofile.Default.Profiles()
    return captureProfileStateResponse{
        Path:              path,
        CustomProfiles:    custom,
        EffectiveProfiles: effective,
        BuiltinCount:      len(builtin),
        CustomCount:       len(custom),
        EffectiveCount:    len(effective),
        UpdatedAt:         updatedAt,
        Capabilities:      captureProfileCapabilitiesValue(),
    }, nil
}

func previewCaptureProfile(profile captureprofile.Profile, observations []captureprofile.Observation) (captureProfilePreviewResponse, error) {
    if len(observations) > maxCaptureProfilePreviewObservations {
        return captureProfilePreviewResponse{}, fmt.Errorf("preview accepts at most %d observations", maxCaptureProfilePreviewObservations)
    }
    registry := captureprofile.NewRegistry(nil)
    if err := registry.Replace([]captureprofile.Profile{profile}); err != nil {
        return captureProfilePreviewResponse{}, err
    }
    normalized := registry.Profiles()[0]
    matches := make([]captureProfilePreviewMatch, 0)
    for index, observation := range observations {
        if match, ok := registry.Match(observation); ok {
            matches = append(matches, captureProfilePreviewMatch{Index: index, Match: match})
        }
    }
    return captureProfilePreviewResponse{
        Profile:      normalized,
        MatchedCount: len(matches),
        Matches:      matches,
    }, nil
}

func handleGetCaptureProfiles(c *gin.Context) {
    state, err := buildCaptureProfileState(captureProfileOverlayPath())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, state)
}

func handlePutCaptureProfiles(c *gin.Context) {
    var request captureProfileUpdateRequest
    if err := c.ShouldBindJSON(&request); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid capture profile payload: " + err.Error()})
        return
    }
    path := captureProfileOverlayPath()
    if err := persistCaptureProfileOverlay(path, request.Profiles); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    state, err := buildCaptureProfileState(path)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, state)
}

func handlePreviewCaptureProfile(c *gin.Context) {
    var request captureProfilePreviewRequest
    if err := c.ShouldBindJSON(&request); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid capture profile preview payload: " + err.Error()})
        return
    }
    response, err := previewCaptureProfile(request.Profile, request.Observations)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, response)
}
''')

write("backend/app/captureprofile_control_test.go", r'''package app

import (
    "agent-ebpf-filter/app/captureprofile"
    "path/filepath"
    "testing"
)

func TestCaptureProfileOverlayPersistsAndPublishes(t *testing.T) {
    path := filepath.Join(t.TempDir(), "profiles.json")
    t.Setenv("AGENT_EBPF_API_PROFILES", path)
    defer func() {
        _ = captureprofile.Default.Replace(captureprofile.BuiltinProfiles())
    }()

    profiles := []captureprofile.Profile{{
        ID: "custom.acme.jobs", Vendor: "acme", Product: "jobs", Operation: "create",
        Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"},
        Directions: []string{"outgoing"}, Methods: []string{"POST"},
        HostSuffixes: []string{"api.acme.test"}, PathPrefixes: []string{"/v9/jobs"}, MinScore: 90,
    }}
    if err := persistCaptureProfileOverlay(path, profiles); err != nil {
        t.Fatal(err)
    }
    state, err := buildCaptureProfileState(path)
    if err != nil {
        t.Fatal(err)
    }
    if state.CustomCount != 1 || state.CustomProfiles[0].ID != "custom.acme.jobs" {
        t.Fatalf("unexpected custom profile state: %+v", state)
    }
    match, ok := captureprofile.Default.Match(captureprofile.Observation{
        Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing",
        Method: "POST", Host: "api.acme.test", Path: "/v9/jobs/42",
    })
    if !ok || match.ProfileID != "custom.acme.jobs" {
        t.Fatalf("published registry did not match custom profile: ok=%v match=%+v", ok, match)
    }
}

func TestPreviewCaptureProfileReturnsMatchingIndexes(t *testing.T) {
    profile := captureprofile.Profile{
        ID: "custom.webhook", Vendor: "acme", Sources: []string{"kernel_socket_prefix"},
        Protocols: []string{"http1"}, Directions: []string{"incoming"},
        Methods: []string{"POST"}, HostSuffixes: []string{"hooks.acme.test"}, MinScore: 70,
    }
    observations := []captureprofile.Observation{
        {Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Host: "hooks.acme.test"},
        {Source: "kernel_socket_prefix", Protocol: "http1", Direction: "incoming", Method: "POST", Host: "hooks.acme.test"},
        {Source: "tls_plaintext", Protocol: "http1", Direction: "incoming", Method: "POST", Host: "hooks.acme.test"},
    }
    response, err := previewCaptureProfile(profile, observations)
    if err != nil {
        t.Fatal(err)
    }
    if response.MatchedCount != 1 || len(response.Matches) != 1 || response.Matches[0].Index != 1 {
        t.Fatalf("unexpected preview response: %+v", response)
    }
}

func TestCaptureProfileOverlayPathUsesConfiguredPath(t *testing.T) {
    path := filepath.Join(t.TempDir(), "managed-profiles.json")
    t.Setenv("AGENT_EBPF_API_PROFILES", path)
    if got := captureProfileOverlayPath(); got != path {
        t.Fatalf("overlay path = %q, want %q", got, path)
    }
}
''')

# Network control-plane routes.
replace_once(
    "backend/app/routes.go",
    '''\tr.GET("/network/interfaces", authMiddleware(), handleNetworkInterfaces)\n''',
    '''\tr.GET("/network/interfaces", authMiddleware(), handleNetworkInterfaces)\n\tr.GET("/network/capture-profiles", authMiddleware(), handleGetCaptureProfiles)\n\tr.PUT("/network/capture-profiles", authMiddleware(), handlePutCaptureProfiles)\n\tr.POST("/network/capture-profiles/preview", authMiddleware(), handlePreviewCaptureProfile)\n''')

# Always run the profile watcher against one canonical overlay file. When no
# environment path is configured, initialize the managed runtime file.
old_watcher = '''func startAPICaptureProfileWatcher(ctx context.Context, jobs *runtimeBackgroundJobs) {\n\tif ctx == nil || jobs == nil {\n\t\treturn\n\t}\n\tpath := strings.TrimSpace(os.Getenv("AGENT_EBPF_API_PROFILES"))\n\tif path == "" {\n\t\treturn\n\t}\n\tif err := captureprofile.ReloadDefaultJSON(path); err != nil {\n\t\tlog.Printf("[WARN] initial API capture profile load failed: %v", err)\n\t} else {\n\t\tlog.Printf("[INFO] API capture profiles loaded from %s", path)\n\t}\n\tjobs.Go(func() {\n\t\tcaptureprofile.WatchDefaultJSON(ctx, path, 2*time.Second, func(err error) {\n\t\t\tlog.Printf("[WARN] API capture profile reload rejected; keeping last known-good rules: %v", err)\n\t\t})\n\t})\n}\n'''
new_watcher = '''func startAPICaptureProfileWatcher(ctx context.Context, jobs *runtimeBackgroundJobs) {\n\tif ctx == nil || jobs == nil {\n\t\treturn\n\t}\n\tpath := captureProfileOverlayPath()\n\tif err := ensureCaptureProfileOverlayFile(path); err != nil {\n\t\tlog.Printf("[WARN] API capture profile control plane unavailable: %v", err)\n\t\treturn\n\t}\n\tif err := captureprofile.ReloadDefaultJSON(path); err != nil {\n\t\tlog.Printf("[WARN] initial API capture profile load failed: %v", err)\n\t} else {\n\t\tlog.Printf("[INFO] API capture profiles loaded from %s", path)\n\t}\n\tjobs.Go(func() {\n\t\tcaptureprofile.WatchDefaultJSON(ctx, path, 2*time.Second, func(err error) {\n\t\t\tlog.Printf("[WARN] API capture profile reload rejected; keeping last known-good rules: %v", err)\n\t\t})\n\t})\n}\n'''
replace_once("backend/app/jobs_background.go", old_watcher, new_watcher)
# jobs_background no longer directly uses strings for the profile watcher, but
# other code may still use it; leave imports untouched and let gofmt/compiler
# tell us if it is now unused.

# ---------------------------------------------------------------------------
# Frontend Socket/Flow visual profile editor.
# ---------------------------------------------------------------------------
write("frontend/src/views/network/SocketCaptureProfiles.vue", r'''<script setup lang="ts">
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
  capabilities: {
    sources: string[];
    protocols: string[];
    directions: string[];
    methods: string[];
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
  min_score: 60,
});

const draft = reactive<CaptureProfile>(emptyProfile());

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
          <div class="meta-line">{{ state?.builtinCount || 0 }} built-in + {{ state?.customCount || 0 }} custom</div>
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
            <span v-if="!record.host_suffixes?.length && !record.path_prefixes?.length && !record.methods?.length" class="meta-line">score-based generic rule</span>
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
''')

# Wire the visual editor into NetworkFlow and flow detail.
replace_once(
    "frontend/src/views/network/NetworkFlow.vue",
    '''  AlertOutlined,\n} from "@ant-design/icons-vue";\n''',
    '''  AlertOutlined,\n  SettingOutlined,\n} from "@ant-design/icons-vue";\n''')
replace_once(
    "frontend/src/views/network/NetworkFlow.vue",
    '''import TrafficGraph from "../../components/network/TrafficGraph.vue";\n''',
    '''import TrafficGraph from "../../components/network/TrafficGraph.vue";\nimport SocketCaptureProfiles from "./SocketCaptureProfiles.vue";\n''')
replace_once(
    "frontend/src/views/network/NetworkFlow.vue",
    '''        <!-- ── Interfaces tab ────────────────────────────────── -->\n        <a-tab-pane key="interfaces">\n''',
    '''        <!-- ── Capture profile tab ───────────────────────────── -->\n        <a-tab-pane key="capture">\n          <template #tab>\n            <span><SettingOutlined /> Capture Profiles</span>\n          </template>\n          <SocketCaptureProfiles :flows="flowList" :selected-flow="selectedFlow" />\n        </a-tab-pane>\n\n        <!-- ── Interfaces tab ────────────────────────────────── -->\n        <a-tab-pane key="interfaces">\n''')
replace_once(
    "frontend/src/views/network/NetworkFlow.vue",
    '''        </a-descriptions>\n      </template>\n    </a-modal>\n''',
    '''        </a-descriptions>\n        <div style="margin-top: 12px; display: flex; justify-content: flex-end">\n          <a-button\n            type="primary"\n            @click="activeTab = 'capture'; showFlowDetail = false"\n          >\n            Create capture profile from this socket\n          </a-button>\n        </div>\n      </template>\n    </a-modal>\n''')

# Documentation for the visual/control-plane workflow.
path = "docs/backend/generic-api-capture.md"
text = read(path)
text += r'''

## Socket-view visual configuration

The Network Flow workspace now owns the capture-profile control plane. Open a
flow/socket detail and choose **Create capture profile from this socket**, or use
the **Capture Profiles** tab directly.

The editor writes one canonical overlay file:

- `AGENT_EBPF_API_PROFILES` when explicitly configured;
- otherwise `${runtime settings dir}/api-capture-profiles.json`.

The file is atomically replaced, validated before publication, and consumed by
the same hot-reload watcher used for external/ConfigMap updates. Built-in
profiles remain the base layer; custom profiles override built-ins with the same
ID.

Authenticated endpoints:

- `GET /network/capture-profiles` — custom/effective profile state and supported
  source/protocol/direction selectors;
- `PUT /network/capture-profiles` — atomically replace the custom overlay;
- `POST /network/capture-profiles/preview` — evaluate one draft against up to
  500 protocol-neutral observations using the production matcher.

The flow preview is intentionally host/method/protocol oriented because the
aggregated socket table does not persist request paths. Path selectors are still
evaluated against real L7 capture events. This keeps the visual configuration
honest instead of fabricating path visibility that the flow aggregator does not
have.
'''
write(path, text)
