<script setup lang="ts">
import axios from "axios";
import { computed, onMounted, onUnmounted, reactive, ref } from "vue";

type CaptureConfig = {
  httpPort: number;
  socks5Port: number;
  socksPort: number;
  interceptTLS: boolean;
  tlsAllowlist: string;
  jsonPreview: boolean;
};
type CaptureFlow = {
  id: number;
  at: string;
  protocol: string;
  host: string;
  method?: string;
  path?: string;
  status?: number;
  durationMs: number;
  contentType?: string;
  requestJson?: string;
  responseJson?: string;
  error?: string;
};
type CaptureStatus = {
  config: CaptureConfig;
  configured: boolean;
  running: boolean;
  caReady: boolean;
  caFingerprint?: string;
  flowCount: number;
};
const config = reactive<CaptureConfig>({
  httpPort: 0,
  socks5Port: 0,
  socksPort: 0,
  interceptTLS: false,
  tlsAllowlist: "",
  jsonPreview: false,
});
const status = ref<CaptureStatus | null>(null);
const flows = ref<CaptureFlow[]>([]);
const selected = ref<CaptureFlow | null>(null);
const busy = ref(false);
const error = ref("");
const showStartConfirm = ref(false);
const showCAConfirm = ref(false);
const showTrustInstructions = ref(false);
const understandStart = ref(false);
const understandCA = ref(false);
const dirty = ref(false);
let polling: ReturnType<typeof setInterval> | undefined;
const canStart = computed(() => !!status.value?.configured && !status.value.running && !dirty.value);
const configuredPorts = computed(() =>
  [config.httpPort, config.socks5Port, config.socksPort].filter((port) => port > 0).length,
);
const errorMessage = (err: unknown) => {
  if (axios.isAxiosError(err)) {
    return String(err.response?.data?.error || err.message);
  }
  return String(err);
};
const load = async (updateForm = false) => {
  try {
    const [state, recent] = await Promise.all([
      axios.get<CaptureStatus>("/network/proxy-capture/status"),
      axios.get<{ flows: CaptureFlow[] }>("/network/proxy-capture/flows"),
    ]);
    status.value = state.data;
    flows.value = recent.data.flows || [];
    if (updateForm || !dirty.value) {
      Object.assign(config, state.data.config);
      dirty.value = false;
    }
  } catch (err) {
    error.value = errorMessage(err);
  }
};
const execute = async (action: () => Promise<unknown>) => {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    await action();
    await load(true);
  } catch (err) {
    error.value = errorMessage(err);
  } finally {
    busy.value = false;
  }
};
const save = () => execute(() =>
  axios.put("/network/proxy-capture/config", { ...config }),
);
const start = async () => {
  if (!understandStart.value) return;
  showStartConfirm.value = false;
  await execute(() =>
    axios.post("/network/proxy-capture/start", { confirm: "I_UNDERSTAND_PROXY_CAPTURE" }),
  );
  understandStart.value = false;
};
const stop = () => execute(() => axios.post("/network/proxy-capture/stop", {}));
const generateCA = async () => {
  if (!understandCA.value) return;
  showCAConfirm.value = false;
  await execute(() =>
    axios.post("/network/proxy-capture/ca/generate", { confirm: "GENERATE_CAPTURE_CA" }),
  );
  understandCA.value = false;
};
const downloadCA = async () => {
  error.value = "";
  try {
    const response = await axios.get("/network/proxy-capture/ca", { responseType: "blob" });
    const objectURL = URL.createObjectURL(response.data as Blob);
    const link = document.createElement("a");
    link.href = objectURL;
    link.download = "capture-ca.pem";
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(objectURL);
  } catch (err) {
    error.value = errorMessage(err);
  }
};
const clear = () => execute(() => axios.delete("/network/proxy-capture/flows"));
const columns = [
  { title: "时间", dataIndex: "at", key: "at", width: 160 },
  { title: "协议", dataIndex: "protocol", key: "protocol", width: 150 },
  { title: "方法", dataIndex: "method", key: "method", width: 90 },
  { title: "域名", dataIndex: "host", key: "host" },
  { title: "路径（不含查询参数）", dataIndex: "path", key: "path" },
  { title: "状态", dataIndex: "status", key: "status", width: 80 },
];
onMounted(() => {
  void load(true);
  polling = setInterval(() => void load(), 2500);
});
onUnmounted(() => {
  if (polling) clearInterval(polling);
});
</script>

<template>
  <a-card title="mitmdump 风格 · 本机 HTTP / SOCKS 上下文捕获" size="small">
    <a-alert
      type="warning"
      show-icon
      message="仅用于你有权监控的流量；默认关闭，不接管系统代理、不安装根证书"
      description="监听地址固定为 127.0.0.1，必须先保存端口并再次确认才会启动。未解密的 SOCKS/CONNECT 仅记录目的地；启用白名单和 CA 后 HTTP/SOCKS CONNECT 的 HTTPS（HTTP/1.1）可解密。"
      style="margin-bottom: 14px"
    />
    <a-alert v-if="error" :message="error" type="error" show-icon closable @close="error = ''" />
    <a-space style="margin-bottom: 12px" wrap>
      <a-tag :color="status?.running ? 'green' : 'default'">{{ status?.running ? "捕获运行中" : "捕获已停止" }}</a-tag>
      <a-tag>{{ status?.flowCount ?? 0 }} 条内存记录 / 最多 300 条</a-tag>
      <a-button size="small" @click="load(true)">刷新</a-button>
    </a-space>
    <a-row :gutter="[14, 14]">
      <a-col :xs="24" :md="8">
        <div>HTTP 正向代理端口（0 = 关闭）</div>
        <a-input-number v-model:value="config.httpPort" :min="0" :max="65535" style="width: 100%" :disabled="status?.running" @change="dirty = true" />
      </a-col>
      <a-col :xs="24" :md="8">
        <div>SOCKS5 CONNECT 端口（0 = 关闭）</div>
        <a-input-number v-model:value="config.socks5Port" :min="0" :max="65535" style="width: 100%" :disabled="status?.running" @change="dirty = true" />
      </a-col>
      <a-col :xs="24" :md="8">
        <div>SOCKS4 / SOCKS4a 端口（0 = 关闭）</div>
        <a-input-number v-model:value="config.socksPort" :min="0" :max="65535" style="width: 100%" :disabled="status?.running" @change="dirty = true" />
      </a-col>
      <a-col :span="24">
        <a-checkbox v-model:checked="config.interceptTLS" :disabled="status?.running" @change="dirty = true">为明确授权的 HTTPS 域名启用 TLS 解密（需先手工信任 CA）</a-checkbox>
      </a-col>
      <a-col v-if="config.interceptTLS" :span="24">
        <div>TLS 解密白名单（每行一项；支持 *.example.com，不接管其他域名）</div>
        <a-textarea v-model:value="config.tlsAllowlist" :rows="3" :disabled="status?.running" @change="dirty = true" placeholder="api.example.com" />
      </a-col>
      <a-col :span="24">
        <a-checkbox v-model:checked="config.jsonPreview" :disabled="status?.running" @change="dirty = true">额外保留小于 4 KiB 的 JSON 请求/响应内容（敏感字段尽力遮蔽，仍可能泄露个人数据）</a-checkbox>
      </a-col>
    </a-row>
    <a-space wrap style="margin-top: 12px">
      <a-button type="primary" :disabled="busy || status?.running || configuredPorts === 0" @click="save">保存端口和捕获配置</a-button>
      <a-button v-if="!status?.running" danger :disabled="busy || !canStart" @click="showStartConfirm = true">确认后启动捕获</a-button>
      <a-button v-else danger :disabled="busy" @click="stop">停止捕获</a-button>
    </a-space>
    <a-divider orientation="left">本地根证书（必须人工确认安装）</a-divider>
    <a-space wrap>
      <a-tag :color="status?.caReady ? 'green' : 'default'">{{ status?.caReady ? "本机代理 CA 已生成" : "未生成代理 CA" }}</a-tag>
      <a-button v-if="!status?.caReady" :disabled="busy || status?.running" @click="showCAConfirm = true">确认后生成本地 CA</a-button>
      <a-button v-if="status?.caReady" @click="downloadCA">下载 CA 公钥证书</a-button>
      <a-button v-if="status?.caReady" @click="showTrustInstructions = true">如何安装系统根证书</a-button>
    </a-space>
    <div v-if="status?.caFingerprint" style="margin-top: 8px; overflow-wrap: anywhere">
      <a-typography-text type="secondary">SHA-256：{{ status.caFingerprint }}</a-typography-text>
    </div>
    <a-typography-paragraph type="secondary" style="margin-top: 6px">
      WebUI 不会静默写入任何系统证书存储区。下载证书后，请在当前设备的系统证书管理器里核对指纹、选择受信任根证书并完成系统权限弹窗；取消即可不安装。
    </a-typography-paragraph>

    <a-divider orientation="left">捕获上下文</a-divider>
    <a-space style="margin-bottom: 8px">
      <a-button size="small" :disabled="!flows.length" @click="clear">清空内存记录</a-button>
    </a-space>
    <a-table :columns="columns" :data-source="flows" :pagination="{ pageSize: 8 }" :row-key="(row: CaptureFlow) => row.id" size="small" :custom-row="(row: CaptureFlow) => ({ onClick: () => { selected = row; } })">
      <template #bodyCell="{ column, record }">
        <span v-if="column.key === 'at'">{{ new Date(record.at).toLocaleTimeString() }}</span>
      </template>
    </a-table>
    <a-drawer :open="!!selected" title="捕获上下文（可能含敏感信息）" width="min(600px, 90vw)" @close="selected = null">
      <pre v-if="selected" style="white-space: pre-wrap; overflow-wrap: anywhere">{{ JSON.stringify(selected, null, 2) }}</pre>
    </a-drawer>

    <a-modal v-model:open="showStartConfirm" title="手动确认启动本机代理捕获" ok-text="确认启动" :ok-button-props="{ disabled: !understandStart || dirty || busy }" @ok="start">
      <p>仅监听 127.0.0.1；开启后，主动把应用配置成该 HTTP/SOCKS 代理才会捕获流量。HTTP 请求上下文可能包含隐私，不应监控非授权软件。</p>
      <a-checkbox v-model:checked="understandStart">我已设置端口，且有权监控这些网络连接。</a-checkbox>
    </a-modal>
    <a-modal v-model:open="showCAConfirm" title="手动确认创建本地根 CA" ok-text="创建" :ok-button-props="{ disabled: !understandCA || busy }" @ok="generateCA">
      <p>将生成新的本地根 CA 和仅保留于服务端的私钥（180 天有效）。创建本身不会安装到系统。私钥若泄露可被用于冒充经过此 CA 信任链验证的网站。</p>
      <a-checkbox v-model:checked="understandCA">我理解根证书的信任风险，并确认自己要创建。</a-checkbox>
    </a-modal>
    <a-modal v-model:open="showTrustInstructions" title="在操作系统中手动信任根证书" :footer="null">
      <p>先下载 capture-ca.pem，核对 SHA-256 指纹。必须由你自己完成系统证书管理器的信任操作：</p>
      <p><strong>Windows：</strong>打开 certlm.msc → 受信任的根证书颁发机构 → 证书 → 导入；选择下载的证书，确认 UAC。</p>
      <p><strong>macOS：</strong>使用“钥匙串访问”导入证书，显式设置“始终信任”并完成密码验证。</p>
      <p><strong>Linux：</strong>使用发行版证书管理器或管理员命令导入系统 CA 存储区；Firefox 等应用可能有独立信任库。</p>
      <p>完成之后可重新打开系统证书管理器撤销信任。仅在需要 HTTPS 调试时才建议安装。</p>
    </a-modal>
  </a-card>
</template>
