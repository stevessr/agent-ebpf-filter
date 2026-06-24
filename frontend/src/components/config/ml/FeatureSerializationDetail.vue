<script setup lang="ts">
import { ref, computed, watch } from "vue";

import {
  SlidersOutlined,
  BranchesOutlined,
  InfoCircleOutlined,
  InteractionOutlined,
} from "@ant-design/icons-vue";

const props = defineProps<{
  modelBaseType: string;
  modelTypeLabel: string;
}>();

// Selected view mode: 'groups' (semantic feature groups) or 'dimensions' (individual 128 dimensions)
const activeView = ref<"groups" | "dimensions">("groups");

// Hovered node ID for GNN topology interaction
const hoveredNodeId = ref<number | null>(null);

// Active flat group ID for flat feature extraction exploration
const activeFlatGroupId = ref<number>(0);

// --- GNN 16 Feature Groups (Nodes) definitions ---
interface GnnNode {
  id: number;
  name: string;
  label: string;
  start: number;
  end: number;
  dim: number;
  description: string;
  details: string;
}

const gnnNodes: GnnNode[] = [
  {
    id: 0,
    name: "behavior_category",
    label: "行为类别独热码",
    start: 0,
    end: 15,
    dim: 15,
    description: "追踪事件的行为分类独热编码",
    details: "包括执行(Exec)、文件读取(Read)、文件写入(Write)、网络连接(Connect)、进程操作(Kill)等 15 种底层 syscall 行为类别的一热向量表示。"
  },
  {
    id: 1,
    name: "process_flags",
    label: "进程属性标志位",
    start: 15,
    end: 22,
    dim: 7,
    description: "当前执行进程的身份属性标记",
    details: "指示是否为 Shell 终端(is_shell)、包管理器(is_package_manager)、系统 Agent(is_agent_cli)及是否以 root 权限运行(is_root)等高危属性判定。"
  },
  {
    id: 2,
    name: "io_flags",
    label: "I/O 结构特征",
    start: 22,
    end: 28,
    dim: 6,
    description: "输入输出重定向及管道链特征",
    details: "编码命令行中是否包含标准输出重定向(>)、追加重定向(>>)、输入重定向(<)、管道链操作(|)、以及是否存在设备文件读写(/dev/)等 I/O 标志。"
  },
  {
    id: 3,
    name: "command_meta",
    label: "命令元数据",
    start: 28,
    end: 32,
    dim: 4,
    description: "命令名称长度与参数总量统计",
    details: "对命令名字符长度、命令行参数(args)个数等信息做归一化(Normalization)后得到的连续数值特征。"
  },
  {
    id: 4,
    name: "arg_statistics",
    label: "参数统计特征",
    start: 32,
    end: 38,
    dim: 6,
    description: "命令行参数的长度分布与熵值",
    details: "包含参数平均长度、参数长度标准差、总参数字节数、以及命令行字符的香农熵(Shannon Entropy)以辅助评估是否包含加密或混淆内容。"
  },
  {
    id: 5,
    name: "sensitive_paths",
    label: "敏感路径命率",
    start: 38,
    end: 48,
    dim: 10,
    description: "对系统敏感关键目录的访问标记",
    details: "包含命令中是否涉及 /etc/, /proc/, /sys/, /var/log/, /root/, /home/, /tmp/, ~/.ssh, ~/.gnupg, /boot/ 等 10 类系统敏感路径的包含状态。"
  },
  {
    id: 6,
    name: "file_extensions",
    label: "文件后缀直方图",
    start: 48,
    end: 58,
    dim: 10,
    description: "命中特定类型文件后缀的统计",
    details: "统计命令参数中以 10 种常见后缀(.go, .py, .js, .ts, .json, .yaml, .toml, .md, .sh, .txt)结尾的文件命中频次及占比分布。"
  },
  {
    id: 7,
    name: "url_patterns",
    label: "网络模式匹配",
    start: 58,
    end: 64,
    dim: 6,
    description: "参数中含有 IP 地址或 URL 的指示器",
    details: "判定命令行中是否包含 HTTP/HTTPS 链接、正则匹配的 IPv4/IPv6 地址、重定向操作计数、以及环境变量引用检测等。"
  },
  {
    id: 8,
    name: "embedding_low",
    label: "语义词嵌入(低阶)",
    start: 64,
    end: 80,
    dim: 16,
    description: "LSH 指令语义编码的低阶维度分量",
    details: "对命令行全长文本进行语义提取(Instruction Embedding)，通过局部敏感哈希(LSH)降维得到的 64 维特征中的前 16 维分量。"
  },
  {
    id: 9,
    name: "embedding_high",
    label: "语义词嵌入(高阶)",
    start: 80,
    end: 96,
    dim: 16,
    description: "LSH 指令语义编码的高阶维度分量",
    details: "局部敏感哈希(LSH)指令语义特征向量中第 17 至 32 维，用于和低阶语义互相校验结合，在潜空间中提供连续表示。"
  },
  {
    id: 10,
    name: "freq_history",
    label: "历史频率指标",
    start: 96,
    end: 104,
    dim: 8,
    description: "滑动窗口中该命令及状态的频次比率",
    details: "包含该命令名在最近 100 次历史事件中的出现频次、历史拦截率(BLOCK)、历史报警率(ALERT)、以及历史检测异常分数的平均值与方差。"
  },
  {
    id: 11,
    name: "pattern_history",
    label: "历史模式指标",
    start: 104,
    end: 112,
    dim: 8,
    description: "历史窗口动作多样性与时间紧凑性",
    details: "包含历史异常分数变化趋势、命令重复爆发强度、特定命令告警率、去重敏感事件占比以及历史记录涉及的用户数与命令回溯时间间距。"
  },
  {
    id: 12,
    name: "temporal",
    label: "实时时序特征",
    start: 112,
    end: 120,
    dim: 8,
    description: "事件产生的流速、PID 密度及时间周期",
    details: "包含每秒捕获事件流速、当前并发 PID 数量、一天中小时的归一化和正余弦周期编码、是否为周末等时序特征。"
  },
  {
    id: 13,
    name: "risk_scores",
    label: "网络风险打分",
    start: 120,
    end: 124,
    dim: 4,
    description: "静态网络风险评判和协议安全分析",
    details: "结合网络抓包上下文进行网络行为审计，综合分析协议、目的 IP、数据量大小得出的基础网络威胁等级特征。"
  },
  {
    id: 14,
    name: "network_anomalies",
    label: "网络异常标志位",
    start: 124,
    end: 128,
    dim: 4,
    description: "是否触发网络高危连接规则",
    details: "指示是否涉及非常规高危端口、潜在反弹 Shell 连接(Reverse Shell)、大流量数据外泄(Data Exfiltration)、以及 DNS 隧道异常等严重安全特征。"
  },
  {
    id: 15,
    name: "global_summary",
    label: "全局概况汇总节点",
    start: 0,
    end: 128,
    dim: 128,
    description: "全维特征线性映射的汇总交互池",
    details: "由 128 维特征整体投影生成的超级聚合节点(Super Node)，在 GNN 每一层消息传递中接收并广播到全网，担当全局记忆库。"
  }
];

// Structural edges defining domain connectivity in GNN
// source, target indices (matching gnnNodes id)
const gnnStaticEdges = [
  [0, 1], [1, 2], [1, 3],
  [2, 5], [2, 6], [2, 7], [2, 14],
  [3, 4], [4, 5], [4, 7],
  [8, 9],
  [10, 11], [11, 12], [10, 12],
  [13, 14], [5, 13], [7, 14],
  [0, 13], [1, 13], [0, 14]
];

// Helper to determine GNN connectivity
const isGnnConnected = (nodeId1: number, nodeId2: number): boolean => {
  if (nodeId1 === nodeId2) return true;
  // Node 15 (global_summary) connects to all nodes (0-14)
  if (nodeId1 === 15 || nodeId2 === 15) return true;
  return gnnStaticEdges.some(
    (e) => (e[0] === nodeId1 && e[1] === nodeId2) || (e[0] === nodeId2 && e[1] === nodeId1)
  );
};

// Get connected nodes list for currently hovered node
const connectedNodes = computed<number[]>(() => {
  if (hoveredNodeId.value === null) return [];
  const list: number[] = [];
  for (let i = 0; i < 16; i++) {
    if (isGnnConnected(hoveredNodeId.value, i)) {
      list.push(i);
    }
  }
  return list;
});

// --- Flat Feature Vector 6 Major Groups (for non-GNN models) ---
interface FlatGroup {
  id: number;
  name: string;
  label: string;
  range: string;
  dim: number;
  color: string;
  description: string;
  features: { index: number; name: string; desc: string }[];
}

const flatGroups: FlatGroup[] = [
  {
    id: 0,
    name: "Command & Process Attributes",
    label: "组 A：命令行与进程属性特征",
    range: "0 - 31",
    dim: 32,
    color: "blue",
    description: "提取捕获事件的 Syscall 类型及底层进程标识的静态/状态属性",
    features: [
      { index: 0, name: "behavior_category[0-14]", desc: "系统行为类别一热码：包含文件读写、权限提升、网络监听、子进程派生等 15 类安全敏感操作" },
      { index: 15, name: "is_shell", desc: "判定执行进程文件名是否为 bash/sh/zsh/fish 等命令行终端 Shell" },
      { index: 16, name: "is_package_manager", desc: "判定执行进程是否为 apt/yum/dnf/pip/npm/bun 等软件包管理器" },
      { index: 17, name: "is_agent_cli", desc: "判断该命令是否来源于 Agent 本身的管理命令或包装命令行" },
      { index: 18, name: "is_root_user", desc: "判定命令发起者是否为 root (UID=0)" },
      { index: 19, name: "has_network_args", desc: "命令行参数中检测到 -p/--port/http 等网络配置与连接参数" },
      { index: 20, name: "has_file_args", desc: "命令行参数中检测到路径格式的字符串 (如 /etc/passwd)" },
      { index: 21, name: "has_redirection", desc: "命令行存在重定向操作符 (>, >>, 2>, &>)" },
      { index: 22, name: "has_pipe", desc: "命令行包含管道操作符 (|)" },
      { index: 23, name: "many_args", desc: "参数个数是否多于 10 个 (高风险指令或脚本注入特征)" },
      { index: 24, name: "dev_access", desc: "命令行中是否读取 /dev/ 设备驱动路径" },
      { index: 25, name: "sudo_in_args", desc: "参数中显式包含 sudo 命令或调用提权" },
      { index: 26, name: "high_confidence", desc: "当前分类器的行为分类判断处于高置信度" },
      { index: 27, name: "medium_confidence", desc: "当前分类器的行为分类判断处于中等置信度" },
      { index: 28, name: "command_len", desc: "命令名字长度 (经过归一化)" },
      { index: 29, name: "args_count", desc: "参数总数统计 (经过归一化)" },
      { index: 30, name: "reserved_30", desc: "保留空闲维度 30" },
      { index: 31, name: "reserved_31", desc: "保留空闲维度 31" }
    ]
  },
  {
    id: 1,
    name: "Argument Statistical Features",
    label: "组 B：参数文本统计与敏感模式特征",
    range: "32 - 63",
    dim: 32,
    color: "cyan",
    description: "深度解析命令行参数的内容分布、文字熵以及特定系统关键路径的命中率",
    features: [
      { index: 32, name: "mean_arg_len", desc: "多个参数的长度均值 (归一化)" },
      { index: 33, name: "std_arg_len", desc: "参数长度分布标准差 (衡量参数异构性)" },
      { index: 34, name: "total_arg_bytes", desc: "整个命令行除去命令名外的总字节数大小" },
      { index: 35, name: "shannon_entropy", desc: "参数文本香农信息熵 (用于检测加密脚本、高随机度文件名或 Base64 负载)" },
      { index: 36, name: "flag_count_ratio", desc: "以 '-' 开头的命令行选项参数(Flags)在总参数中的占比" },
      { index: 37, name: "pos_count_ratio", desc: "位置参数(Positional args, 如目标文件/IP)在总参数中的占比" },
      { index: 38, name: "contains_etc", desc: "参数包含系统配置路径 /etc/ 访问" },
      { index: 39, name: "contains_proc", desc: "参数包含进程状态目录 /proc/ 访问" },
      { index: 40, name: "contains_sys", desc: "参数包含内核系统目录 /sys/ 访问" },
      { index: 41, name: "contains_var_log", desc: "参数包含日志管理目录 /var/log/ 访问" },
      { index: 42, name: "contains_root_home", desc: "参数包含 root 超级用户主目录 /root/ 访问" },
      { index: 43, name: "contains_home", desc: "参数包含普通用户主目录 /home/ 访问" },
      { index: 44, name: "contains_tmp", desc: "参数包含临时运行目录 /tmp/ 访问" },
      { index: 45, name: "contains_ssh", desc: "参数包含 ~/.ssh 安全密钥目录访问" },
      { index: 46, name: "contains_gnupg", desc: "参数包含 ~/.gnupg 密钥环管理目录访问" },
      { index: 47, name: "contains_boot", desc: "参数包含启动引导目录 /boot/ 访问" },
      { index: 48, name: "ext_go", desc: "参数包含 .go 后缀 Go 源码文件" },
      { index: 49, name: "ext_py", desc: "参数包含 .py 后缀 Python 脚本文件" },
      { index: 50, name: "ext_js", desc: "参数包含 .js 后缀 Node.js 代码文件" },
      { index: 51, name: "ext_ts", desc: "参数包含 .ts 后缀 TypeScript 代码文件" },
      { index: 52, name: "ext_json", desc: "参数包含 .json 配置文件" },
      { index: 53, name: "ext_yaml", desc: "参数包含 .yaml/.yml 服务配置文件" },
      { index: 54, name: "ext_toml", desc: "参数包含 .toml 包管理器配置文件" },
      { index: 55, name: "ext_md", desc: "参数包含 .md 文档文件" },
      { index: 56, name: "ext_sh", desc: "参数包含 .sh Shell 脚本批处理文件" },
      { index: 57, name: "ext_txt", desc: "参数包含 .txt 纯文本文件" },
      { index: 58, name: "has_url_pattern", desc: "参数匹配包含 HTTP/HTTPS 等网络下载链接特征" },
      { index: 59, name: "has_ip_pattern", desc: "参数匹配包含合规格式的 IPv4 或 IPv6 网络目标地址" },
      { index: 60, name: "redirect_count", desc: "命令行重定向次数统计" },
      { index: 61, name: "pipe_count", desc: "命令行管道连接层数统计" },
      { index: 62, name: "unique_args_ratio", desc: "重复参数去重比，越低说明参数包含高度同质冗余内容" },
      { index: 63, name: "has_env_var", desc: "参数中涉及对系统环境变量的读取与引用 ($PATH 等)" }
    ]
  },
  {
    id: 2,
    name: "Instruction Embeddings",
    label: "组 C：指令语义映射向量 (Embeddings)",
    range: "64 - 95",
    dim: 32,
    color: "purple",
    description: "通过局部敏感哈希(LSH)神经网络在语义隐空间中将全命令行编码得到的哈希表示前 32 维",
    features: [
      { index: 64, name: "semantic_lsh[0]", desc: "低维命令哈希投射值，描述命令名基本词根含义" },
      { index: 72, name: "semantic_lsh[8]", desc: "中维词义敏感特征，区分操作偏好" },
      { index: 80, name: "semantic_lsh[16]", desc: "中高维参数结构特征，提取参数的句法角色" },
      { index: 88, name: "semantic_lsh[24]", desc: "高维语义组合分量，用于建模复杂语义联合表示" }
    ]
  },
  {
    id: 3,
    name: "Recent History Aggregates",
    label: "组 D：滑动窗口近期行为历史统计",
    range: "96 - 111",
    dim: 16,
    color: "orange",
    description: "记录该进程组在过去滑动窗口(如最近 100 次拦截或放行事件)中的时序指标聚合",
    features: [
      { index: 96, name: "comm_match_ratio", desc: "相同命令在历史滑动窗口中的出现概率" },
      { index: 97, name: "history_block_rate", desc: "该进程以往触发 BLOCK (拦截)动作的比率" },
      { index: 98, name: "history_alert_rate", desc: "该进程以往触发 ALERT (告警)动作的比率" },
      { index: 99, name: "mean_anomaly_score", desc: "历史滑动窗口内平均异常判定分数" },
      { index: 100, name: "anomaly_variance", desc: "异常分数的波动差值，高方差意味着行为极度不稳定" },
      { index: 101, name: "category_diversity", desc: "历史记录中观察到的行为种类的多样性" },
      { index: 102, name: "anomaly_trend", desc: "异常值近半段与远半段的差值趋势，呈上升说明危害性可能加剧" },
      { index: 103, name: "time_weighted_activity", desc: "最近 5 次操作在该进程所属链条中的比率" },
      { index: 104, name: "comm_repetition_burst", desc: "瞬间高频重复执行相同命令的连发强度" },
      { index: 105, name: "comm_specific_alert_rate", desc: "本命令独有的历史告警概率" },
      { index: 106, name: "sensitive_event_ratio", desc: "历史事件中涉及敏感文件操作或强行终止进程的占比" },
      { index: 107, name: "network_event_ratio", desc: "历史事件中网络连接类行为的占比" },
      { index: 108, name: "root_event_ratio", desc: "历史事件以超级管理员权限启动的比例" },
      { index: 109, name: "distinct_comms", desc: "滑动窗口中去重命令名的数量 (判断命令多元程度)" },
      { index: 110, name: "temporal_recency", desc: "与上次运行该命令之间的时间长度 (1/(1+dt))" },
      { index: 111, name: "distinct_users", desc: "在滑动窗口事件中观测到的不同系统用户名总数" }
    ]
  },
  {
    id: 4,
    name: "Event Velocity & Temporal Features",
    label: "组 E：捕获速率与时序状态统计",
    range: "112 - 119",
    dim: 8,
    color: "green",
    description: "评估主机整体受控操作的流速强度以及执行时所处的时间尺度",
    features: [
      { index: 112, name: "event_velocity", desc: "全局事件秒级到达流速 (反映是否处于暴力写入或扫描态)" },
      { index: 113, name: "distinct_pids_density", desc: "近期活跃进程 PID 密度，值较高通常伴随着多线程并发攻击" },
      { index: 114, name: "hour_of_day", desc: "一天中小时的归一化表示 (0 - 1.0)" },
      { index: 115, name: "day_of_week", desc: "一周中工作日的归一化表示 (0 - 1.0)" },
      { index: 116, name: "sin_hour", desc: "小时的 Sine 投影，解决 23点 和 0点 在坐标轴上的断崖式脱节" },
      { index: 117, name: "cos_hour", desc: "小时的 Cosine 投影，与正弦投影组成二维环形时间特征" },
      { index: 118, name: "is_weekend", desc: "判定运行时间是否处于周六或周日 (非工作时间访问敏感设备风险提高)" },
      { index: 119, name: "mean_historical_args_len", desc: "历史全部参数的平均长度聚合值" }
    ]
  },
  {
    id: 5,
    name: "Network Audit Features",
    label: "组 F：网络深度流审计安全标志",
    range: "120 - 127",
    dim: 8,
    color: "red",
    description: "由网络数据包探针提取的网络交互异常分析，映射高危害性的外发通信",
    features: [
      { index: 120, name: "net_risk_score", desc: "基于协议复杂度和敏感位置计算的实时网络审计评分" },
      { index: 121, name: "suspicious_port_flag", desc: "目标端口在非标应用定义之列 (例如 HTTP 采用 4444 端口)" },
      { index: 122, name: "reverse_shell_flag", desc: "通过输入输出管道建立远程交互式 Shell 强关联 (反弹 Shell 极高危特征)" },
      { index: 123, name: "data_exfil_flag", desc: "短时间、单向大字节输出特征，疑似敏感文件被批量拷贝发送" },
      { index: 124, name: "dns_tunnel_flag", desc: "检测到目的域名长度及频率极度异常，疑似走 DNS 掩护隧道泄漏数据" },
      { index: 125, name: "cleartext_proto_flag", desc: "检测到使用了易被嗅探的明文通信协议 (如 FTP/Telnet/Plain HTTP)" },
      { index: 126, name: "unusual_target_flag", desc: "目标主机 IP 处于地理黑名单或从未交互过的陌生外部自治域" },
      { index: 127, name: "port_scan_flag", desc: "进程快速发起连续 IP 同一端口或单一 IP 多端口连接测试 (端口扫描行为)" }
    ]
  }
];

const selectedNodeDetails = computed<GnnNode | null>(() => {
  if (hoveredNodeId.value === null) return null;
  return gnnNodes.find((n) => n.id === hoveredNodeId.value) || null;
});

const isGnnModel = computed(() => props.modelBaseType === "graph_learning");

// Toggle dynamic active tabs
watch(
  () => props.modelBaseType,
  (newBase) => {
    if (newBase === "graph_learning") {
      activeView.value = "groups";
    } else {
      activeView.value = "dimensions";
    }
  },
  { immediate: true }
);
</script>

<template>
  <a-col :xs="24" style="margin-top: 16px">
    <a-card size="small" class="serialization-card">
      <template #title>
        <div class="card-header-title">
          <SlidersOutlined class="header-icon" />
          <span>特征序列化布局与还原 (Feature Serialization)</span>
          <a-tag color="processing" style="margin-left: 8px">
            {{ isGnnModel ? "图神经网络拓扑" : "平铺 128维 向量" }}
          </a-tag>
        </div>
      </template>

      <!-- Description Header -->
      <div class="description-section">
        <a-alert type="info" show-icon class="glossy-alert">
          <template #message>
            <div style="font-weight: 600">从内核探针到模型特征还原的序列化流水线</div>
          </template>
          <template #description>
            <div class="pipeline-desc">
              Linux 运行期间，当代理执行高危操作时，系统的 <b>eBPF</b> 拦截器与包装探针会捕获执行详情（程序名、参数数组、环境变量、历史活动周期、网络状态及传输元数据），并在网关侧将其还原、编码序列化为 <b>128个平面维度浮点特征（Feature Vector）</b>。<br />
              <div v-if="isGnnModel" style="margin-top: 4px">
                在当前选中的 <b>GNN (图神经网络模型)</b> 中，这 128 维平面特征将被<b>重塑 (Reshape) 映射至 16 个特征子图节点</b>，并通过 domain-specific 的图结构关系传递消息（Message Passing）学习高维交互作用特征。
              </div>
              <div v-else style="margin-top: 4px">
                在当前选中的 <b>{{ modelTypeLabel }}</b> 模型中，特征向量保持 128 维平铺（Flat）格式输入分类器中，由决策森林或线性决策面判定高危概率。
              </div>
            </div>
          </template>
        </a-alert>
      </div>

      <!-- Tab Buttons -->
      <div class="view-toggle-bar">
        <a-radio-group v-model:value="activeView" button-style="solid" size="small">
          <a-radio-button value="groups" v-if="isGnnModel">
            <BranchesOutlined /> 16个 GNN 交互节点重塑
          </a-radio-button>
          <a-radio-button value="dimensions">
            <SlidersOutlined /> 128维原始特征拆解
          </a-radio-button>
        </a-radio-group>
      </div>

      <!-- GNN GROUP REDIRECT INTERACTIVE DISPLAY -->
      <div v-if="activeView === 'groups' && isGnnModel" class="gnn-interactive-container">
        <a-row :gutter="[16, 16]">
          <!-- Grid of GNN Nodes -->
          <a-col :xs="24" :lg="16">
            <div class="gnn-title">
              <span><b>GNN 特征重塑节点拓扑</b> (鼠标悬停某一节点以查看其信息传递通路)</span>
            </div>
            
            <div class="gnn-grid">
              <div
                v-for="node in gnnNodes"
                :key="node.id"
                class="gnn-node-pill"
                :class="{
                  'active': hoveredNodeId === node.id,
                  'connected': hoveredNodeId !== null && hoveredNodeId !== node.id && isGnnConnected(hoveredNodeId, node.id),
                  'dimmed': hoveredNodeId !== null && hoveredNodeId !== node.id && !isGnnConnected(hoveredNodeId, node.id),
                  'global-super-node': node.id === 15
                }"
                @mouseenter="hoveredNodeId = node.id"
                @mouseleave="hoveredNodeId = null"
              >
                <div class="node-id">#{{ node.id }}</div>
                <div class="node-name">{{ node.name }}</div>
                <div class="node-label">{{ node.label }}</div>
                <div class="node-badge-row">
                  <span class="badge range">F[{{ node.start }}-{{ node.end - 1 }}]</span>
                  <span class="badge dim">{{ node.dim }} 维</span>
                </div>
              </div>
            </div>
          </a-col>

          <!-- Interaction details -->
          <a-col :xs="24" :lg="8">
            <div class="gnn-side-panel">
              <template v-if="selectedNodeDetails">
                <div class="panel-header">
                  <span class="panel-node-id">Node #{{ selectedNodeDetails.id }}</span>
                  <h4 class="panel-node-title">{{ selectedNodeDetails.label }}</h4>
                  <code class="panel-node-code">{{ selectedNodeDetails.name }}</code>
                </div>
                
                <div class="panel-section">
                  <div class="section-title">原始特征片段索引</div>
                  <div class="index-box">
                    <span>区间索引: <b>{{ selectedNodeDetails.start }} - {{ selectedNodeDetails.end - 1 }}</b></span>
                    <span style="margin-left: 16px">所含维度: <b>{{ selectedNodeDetails.dim }} 维</b></span>
                  </div>
                </div>

                <div class="panel-section">
                  <div class="section-title">功能简述</div>
                  <p class="section-text">{{ selectedNodeDetails.description }}</p>
                </div>

                <div class="panel-section">
                  <div class="section-title">序列化细节</div>
                  <p class="section-text details-text">{{ selectedNodeDetails.details }}</p>
                </div>

                <div class="panel-section">
                  <div class="section-title">
                    <InteractionOutlined /> 
                    <span>GNN 消息传播通路 (层内相连节点)</span>
                  </div>
                  <div class="connection-pills">
                    <div 
                      v-for="cid in connectedNodes" 
                      :key="cid" 
                      class="conn-pill"
                      :class="{ 'super': cid === 15 }"
                      @mouseenter="hoveredNodeId = cid"
                    >
                      #{{ cid }} {{ gnnNodes[cid].name }}
                    </div>
                  </div>
                </div>
              </template>
              <div v-else class="empty-panel">
                <InfoCircleOutlined class="empty-icon" />
                <p>将鼠标指针悬停左侧 GNN 节点卡片上，即可动态分析其特征在消息传递阶段的网络连接通路与序列化内容。</p>
              </div>
            </div>
          </a-col>
        </a-row>

        <a-divider style="margin: 16px 0" />

        <!-- GNN Nodes Table -->
        <div style="margin-top: 8px">
          <div class="table-title">16个 GNN 特征节点序列化映射总览</div>
          <table class="custom-flat-table">
            <thead>
              <tr>
                <th style="width: 80px">节点 ID</th>
                <th style="width: 160px">特征组代码</th>
                <th style="width: 150px">映射名称</th>
                <th style="width: 120px">128维切片索引</th>
                <th style="width: 70px">子维数</th>
                <th>被序列化的原始特征内容</th>
              </tr>
            </thead>
            <tbody>
              <tr 
                v-for="node in gnnNodes" 
                :key="node.id"
                :class="{ 'highlight-row': hoveredNodeId === node.id }"
                @mouseenter="hoveredNodeId = node.id"
                @mouseleave="hoveredNodeId = null"
              >
                <td><a-tag :color="node.id === 15 ? 'purple' : 'blue'">#{{ node.id }}</a-tag></td>
                <td><code>{{ node.name }}</code></td>
                <td><b>{{ node.label }}</b></td>
                <td><code>F[{{ node.start }} - {{ node.end - 1 }}]</code></td>
                <td>{{ node.dim }}</td>
                <td style="font-size: 12px; color: #666">{{ node.details }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- FLAT DIMENSIONS BREAKDOWN -->
      <div v-if="activeView === 'dimensions' || !isGnnModel" class="flat-dimensions-container">
        <a-row :gutter="[16, 16]">
          <!-- Left side: 6 major groups selection -->
          <a-col :xs="24" :md="8">
            <div class="flat-sidebar">
              <div 
                v-for="group in flatGroups" 
                :key="group.id"
                class="flat-group-item"
                :class="{ 'active': activeFlatGroupId === group.id }"
                @click="activeFlatGroupId = group.id"
              >
                <div class="group-header">
                  <a-tag :color="group.color" class="group-tag">Index {{ group.range }}</a-tag>
                  <span class="group-dim">{{ group.dim }} 维</span>
                </div>
                <div class="group-title">{{ group.label }}</div>
                <div class="group-desc">{{ group.description }}</div>
              </div>
            </div>
          </a-col>

          <!-- Right side: Feature list in group -->
          <a-col :xs="24" :md="16">
            <div class="flat-feature-panel">
              <div class="panel-title-row">
                <a-tag :color="flatGroups[activeFlatGroupId].color" style="font-size: 13px; padding: 4px 8px">
                  Index {{ flatGroups[activeFlatGroupId].range }}
                </a-tag>
                <span class="panel-title-text">{{ flatGroups[activeFlatGroupId].label }}</span>
                <span class="panel-subtitle-dim">包含特征数: {{ flatGroups[activeFlatGroupId].dim }}</span>
              </div>

              <div class="features-scroll-area">
                <table class="custom-flat-table">
                  <thead>
                    <tr>
                      <th style="width: 100px">向量位置 Index</th>
                      <th style="width: 200px">特征变量名</th>
                      <th>序列化规则与安全含义</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="feat in flatGroups[activeFlatGroupId].features" :key="feat.index">
                      <td><a-tag color="default" style="font-family: monospace">F[{{ feat.index }}]</a-tag></td>
                      <td><code style="color: #c41d7f">{{ feat.name }}</code></td>
                      <td style="font-size: 12px; color: #555">{{ feat.desc }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </a-col>
        </a-row>
      </div>
    </a-card>
  </a-col>
</template>

<style scoped>
.serialization-card {
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04);
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.card-header-title {
  display: flex;
  align-items: center;
  font-size: 15px;
  font-weight: 600;
}

.header-icon {
  margin-right: 8px;
  color: #1890ff;
}

.description-section {
  margin-bottom: 16px;
}

.glossy-alert {
  background: rgba(24, 144, 255, 0.04);
  border: 1px dashed rgba(24, 144, 255, 0.2);
  border-radius: 6px;
}

.pipeline-desc {
  font-size: 12px;
  line-height: 1.7;
  color: #555;
}

.view-toggle-bar {
  margin-bottom: 16px;
  display: flex;
  justify-content: flex-start;
}

/* GNN Grid layout styles */
.gnn-title {
  font-size: 13px;
  color: #666;
  margin-bottom: 12px;
}

.gnn-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
  max-height: 520px;
  overflow-y: auto;
  padding: 4px;
}

.gnn-node-pill {
  background: #fdfdfd;
  border: 1px solid #e8e8e8;
  border-radius: 6px;
  padding: 10px;
  cursor: pointer;
  position: relative;
  transition: all 0.3s cubic-bezier(0.25, 0.8, 0.25, 1);
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.02);
}

.gnn-node-pill:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 10px rgba(0, 0, 0, 0.08);
}

.gnn-node-pill.active {
  border-color: #1890ff;
  background: rgba(24, 144, 255, 0.06);
  box-shadow: 0 0 8px rgba(24, 144, 255, 0.3);
  z-index: 10;
}

.gnn-node-pill.connected {
  border-color: #52c41a;
  background: rgba(82, 196, 26, 0.04);
  border-style: dashed;
}

.gnn-node-pill.dimmed {
  opacity: 0.45;
  filter: grayscale(40%);
}

.gnn-node-pill.global-super-node {
  background: #faf7ff;
  border-color: #d3adf7;
}

.gnn-node-pill.global-super-node.active {
  border-color: #722ed1;
  background: rgba(114, 46, 209, 0.08);
  box-shadow: 0 0 8px rgba(114, 46, 209, 0.3);
}

.node-id {
  font-size: 11px;
  font-weight: 700;
  color: #999;
}

.gnn-node-pill.active .node-id {
  color: #1890ff;
}

.gnn-node-pill.connected .node-id {
  color: #52c41a;
}

.gnn-node-pill.global-super-node .node-id {
  color: #722ed1;
}

.node-name {
  font-size: 12px;
  font-family: monospace;
  font-weight: 600;
  color: #333;
  margin-top: 2px;
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
}

.node-label {
  font-size: 11px;
  color: #666;
  margin-top: 4px;
}

.node-badge-row {
  margin-top: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.badge {
  font-size: 9px;
  padding: 1px 4px;
  border-radius: 3px;
}

.badge.range {
  background: #e6f7ff;
  color: #0050b3;
  font-family: monospace;
}

.badge.dim {
  background: #f5f5f5;
  color: #555;
}

/* Side panel info details */
.gnn-side-panel {
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
  padding: 16px;
  height: 100%;
  min-height: 380px;
  display: flex;
  flex-direction: column;
  box-shadow: inset 0 0 10px rgba(0, 0, 0, 0.01);
}

.panel-header {
  border-bottom: 1px solid #e8e8e8;
  padding-bottom: 12px;
  margin-bottom: 12px;
}

.panel-node-id {
  font-size: 11px;
  font-weight: bold;
  color: #1890ff;
  text-transform: uppercase;
}

.panel-node-title {
  font-size: 16px;
  font-weight: 600;
  margin: 4px 0 2px 0;
  color: #222;
}

.panel-node-code {
  font-size: 11px;
  color: #c41d7f;
  background: #fff0f6;
  padding: 2px 6px;
  border-radius: 3px;
}

.panel-section {
  margin-bottom: 12px;
}

.section-title {
  font-size: 12px;
  font-weight: bold;
  color: #888;
  margin-bottom: 6px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.index-box {
  background: #fff;
  border: 1px solid #e8e8e8;
  border-radius: 4px;
  padding: 6px 10px;
  font-size: 12px;
  font-family: monospace;
  color: #333;
}

.section-text {
  font-size: 12px;
  line-height: 1.5;
  color: #444;
  margin: 0;
}

.details-text {
  background: rgba(0, 0, 0, 0.01);
  padding: 6px 8px;
  border-left: 3px solid #1890ff;
  font-style: normal;
  color: #666;
}

.connection-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 4px;
}

.conn-pill {
  font-size: 10px;
  padding: 3px 8px;
  border-radius: 12px;
  background: #f6ffed;
  color: #389e0d;
  border: 1px solid #b7eb8f;
  cursor: pointer;
  transition: all 0.2s;
  font-family: monospace;
}

.conn-pill:hover {
  background: #389e0d;
  color: #fff;
}

.conn-pill.super {
  background: #f9f0ff;
  color: #531dab;
  border-color: #d3adf7;
}

.conn-pill.super:hover {
  background: #531dab;
  color: #fff;
}

.empty-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #999;
  text-align: center;
  padding: 40px 10px;
}

.empty-icon {
  font-size: 32px;
  color: #d9d9d9;
  margin-bottom: 12px;
}

.empty-panel p {
  font-size: 12px;
  line-height: 1.6;
  margin: 0;
  max-width: 220px;
}

/* Flat dimensions breakdown layout */
.flat-dimensions-container {
  margin-top: 8px;
}

.flat-sidebar {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 480px;
  overflow-y: auto;
  padding-right: 4px;
}

.flat-group-item {
  background: #fff;
  border: 1px solid #e8e8e8;
  border-radius: 6px;
  padding: 12px;
  cursor: pointer;
  transition: all 0.3s;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.02);
}

.flat-group-item:hover {
  border-color: #1890ff;
  background: rgba(24, 144, 255, 0.01);
}

.flat-group-item.active {
  border-color: #1890ff;
  background: rgba(24, 144, 255, 0.04);
  box-shadow: 0 2px 8px rgba(24, 144, 255, 0.1);
}

.group-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}

.group-tag {
  font-size: 10px;
}

.group-dim {
  font-size: 11px;
  color: #999;
  font-weight: 500;
}

.group-title {
  font-size: 13px;
  font-weight: 600;
  color: #333;
  margin-bottom: 4px;
}

.group-desc {
  font-size: 11px;
  color: #777;
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.flat-feature-panel {
  background: #fff;
  border: 1px solid #e8e8e8;
  border-radius: 8px;
  padding: 16px;
  height: 100%;
}

.panel-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  border-bottom: 1px solid #e8e8e8;
  padding-bottom: 12px;
  margin-bottom: 12px;
}

.panel-title-text {
  font-size: 14px;
  font-weight: 600;
  color: #222;
}

.panel-subtitle-dim {
  font-size: 11px;
  color: #999;
  margin-left: auto;
}

.features-scroll-area {
  max-height: 380px;
  overflow-y: auto;
}

/* Custom layout table formatting */
.table-title {
  font-size: 13px;
  font-weight: 600;
  color: #444;
  margin-bottom: 8px;
}

.custom-flat-table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
}

.custom-flat-table th {
  background: #f5f5f5;
  color: #555;
  font-weight: 600;
  font-size: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid #e8e8e8;
}

.custom-flat-table td {
  padding: 8px 12px;
  border-bottom: 1px solid #f0f0f0;
  font-size: 12px;
  transition: background-color 0.2s;
}

.custom-flat-table tr:hover td {
  background: rgba(24, 144, 255, 0.02);
}

.custom-flat-table tr.highlight-row td {
  background: rgba(24, 144, 255, 0.04);
}

.custom-flat-table tr.highlight-row td:first-child {
  border-left: 3px solid #1890ff;
}

code {
  font-family: Consolas, "Liberation Mono", Courier, monospace;
}
</style>
