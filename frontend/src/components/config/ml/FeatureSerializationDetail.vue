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

// Active flat group ID for flat feature extraction exploration
const activeFlatGroupId = ref<number>(0);

// --- Flat Feature Vector 6 Major Groups (for non-GNN models) ---
interface FlatFeature {
  index: number;
  name: string;
  desc: string;
}

interface FlatGroup {
  id: number;
  name: string;
  label: string;
  range: string;
  dim: number;
  color: string;
  description: string;
  features: FlatFeature[];
}

const flatGroups = computed<FlatGroup[]>(() => {
  return [
    {
      id: 0,
      name: "Command & Process Attributes",
      label: "组 A：命令行与进程属性特征",
      range: "0 - 31",
      dim: 32,
      color: "blue",
      description: "提取捕获事件的 Syscall 类型及底层进程标识的静态/状态属性",
      features: [
        {
          index: 0,
          name: "behavior_category[UNKNOWN]",
          desc: "未识别/默认系统行为",
        },
        {
          index: 1,
          name: "behavior_category[FILE_READ]",
          desc: "文件读取操作行为 (如 cat, less, hexdump)",
        },
        {
          index: 2,
          name: "behavior_category[FILE_WRITE]",
          desc: "文件写入/修改操作行为 (如 cp, tee, touch)",
        },
        {
          index: 3,
          name: "behavior_category[FILE_DELETE]",
          desc: "文件删除/擦除操作行为 (如 rm, shred, unlink)",
        },
        {
          index: 4,
          name: "behavior_category[FILE_PERMISSION]",
          desc: "更改权限或属主/ACL操作行为 (如 chmod, chown)",
        },
        {
          index: 5,
          name: "behavior_category[NETWORK]",
          desc: "网络发起连接/请求行为 (如 curl, wget, nc, tcpdump)",
        },
        {
          index: 6,
          name: "behavior_category[PROCESS_EXEC]",
          desc: "进程启动或 Shell 命令行执行 (如 exec, bash, sh)",
        },
        {
          index: 7,
          name: "behavior_category[PROCESS_KILL]",
          desc: "进程强制杀死/终止行为 (如 kill, killall, pkill)",
        },
        {
          index: 8,
          name: "behavior_category[SYSTEM_INFO]",
          desc: "主机指标/元数据查询操作 (如 uptime, df, env, ss)",
        },
        {
          index: 9,
          name: "behavior_category[PACKAGE_MANAGER]",
          desc: "包管理器安装/卸载软件行为 (如 apt, npm, pip, gem)",
        },
        {
          index: 10,
          name: "behavior_category[DATABASE]",
          desc: "数据库控制台访问或备份行为 (如 mysql, redis-cli)",
        },
        {
          index: 11,
          name: "behavior_category[COMPRESSION]",
          desc: "文件解压缩或打包归档行为 (如 tar, zip, gzip)",
        },
        {
          index: 12,
          name: "behavior_category[DEVELOPMENT]",
          desc: "软件编译构建与版本管理操作 (如 git, gcc, node)",
        },
        {
          index: 13,
          name: "behavior_category[CONTAINER]",
          desc: "虚拟化容器与集群管理行为 (如 docker, kubectl)",
        },
        {
          index: 14,
          name: "behavior_category[SENSITIVE]",
          desc: "高危管理/提权/内核模块操作 (如 sudo, passwd, modprobe)",
        },
        {
          index: 15,
          name: "is_shell",
          desc: "判定执行程序文件名是否为常用终端 Shell 解释器",
        },
        {
          index: 16,
          name: "is_package_manager",
          desc: "判定执行程序是否为包管理器或软件安装运行程序",
        },
        {
          index: 17,
          name: "is_agent_cli",
          desc: "判断该命令是否来源于 Agent 本身的管理命令或包装命令行",
        },
        {
          index: 18,
          name: "is_root_user",
          desc: "判定命令发起者是否为超级管理员 root (UID=0)",
        },
        {
          index: 19,
          name: "has_network_args",
          desc: "命令行参数中检测到 -p/--port/http/IP 等网络配置选项",
        },
        {
          index: 20,
          name: "has_file_args",
          desc: "命令行参数中检测到路径格式的字符串 (如 /etc/passwd 等)",
        },
        {
          index: 21,
          name: "has_redirection",
          desc: "命令行存在重定向操作符 (>, >>, 2>, &>)",
        },
        { index: 22, name: "has_pipe", desc: "命令行包含管道操作符 (|)" },
        {
          index: 23,
          name: "many_args",
          desc: "参数个数是否多于 10 个 (高风险指令或脚本注入特征)",
        },
        {
          index: 24,
          name: "dev_access",
          desc: "命令行中是否读取 /dev/ 路径 (涉及直连硬件或特殊设备)",
        },
        {
          index: 25,
          name: "sudo_in_args",
          desc: "参数中显式包含 sudo 提权命令",
        },
        {
          index: 26,
          name: "high_confidence",
          desc: "当前分类器的行为分类判断处于高置信度",
        },
        {
          index: 27,
          name: "medium_confidence",
          desc: "当前分类器的行为分类判断处于中等置信度",
        },
        { index: 28, name: "command_len", desc: "命令名字的归一化长度特征" },
        { index: 29, name: "args_count", desc: "参数个数归一化数值" },
        { index: 30, name: "reserved_30", desc: "保留空闲特征维度 30" },
        { index: 31, name: "reserved_31", desc: "保留空闲特征维度 31" },
      ],
    },
    {
      id: 1,
      name: "Argument Statistical Features",
      label: "组 B：参数文本统计与敏感模式特征",
      range: "32 - 63",
      dim: 32,
      color: "cyan",
      description:
        "深度解析命令行参数的内容分布、文字熵以及特定系统关键路径的命中率",
      features: [
        {
          index: 32,
          name: "mean_arg_len",
          desc: "多个参数的长度均值 (归一化)",
        },
        {
          index: 33,
          name: "std_arg_len",
          desc: "参数长度分布标准差 (衡量参数异构性)",
        },
        {
          index: 34,
          name: "total_arg_bytes",
          desc: "整个命令行除去命令名外的总字节数大小",
        },
        {
          index: 35,
          name: "shannon_entropy",
          desc: "参数文本香农信息熵 (用于检测加密脚本、高随机度文件名或 Base64 负载)",
        },
        {
          index: 36,
          name: "flag_count_ratio",
          desc: "以 '-' 开头的命令行选项参数(Flags)在总参数中的占比",
        },
        {
          index: 37,
          name: "pos_count_ratio",
          desc: "位置参数(Positional args)在总参数中的占比",
        },
        {
          index: 38,
          name: "contains_etc",
          desc: "参数包含系统配置路径 /etc/ 访问",
        },
        {
          index: 39,
          name: "contains_proc",
          desc: "参数包含进程状态目录 /proc/ 访问",
        },
        {
          index: 40,
          name: "contains_sys",
          desc: "参数包含内核系统目录 /sys/ 访问",
        },
        {
          index: 41,
          name: "contains_var_log",
          desc: "参数包含日志管理目录 /var/log/ 访问",
        },
        {
          index: 42,
          name: "contains_root_home",
          desc: "参数包含 root 超级用户主目录 /root/ 访问",
        },
        {
          index: 43,
          name: "contains_home",
          desc: "参数包含普通用户主目录 /home/ 访问",
        },
        {
          index: 44,
          name: "contains_tmp",
          desc: "参数包含临时运行目录 /tmp/ 访问",
        },
        {
          index: 45,
          name: "contains_ssh",
          desc: "参数包含 ~/.ssh 安全密钥目录访问",
        },
        {
          index: 46,
          name: "contains_gnupg",
          desc: "参数包含 ~/.gnupg 密钥环管理目录访问",
        },
        {
          index: 47,
          name: "contains_boot",
          desc: "参数包含启动引导目录 /boot/ 访问",
        },
        { index: 48, name: "ext_go", desc: "参数包含 .go 后缀 Go 源码文件" },
        {
          index: 49,
          name: "ext_py",
          desc: "参数包含 .py 后缀 Python 脚本文件",
        },
        {
          index: 50,
          name: "ext_js",
          desc: "参数包含 .js 后缀 Node.js 代码文件",
        },
        {
          index: 51,
          name: "ext_ts",
          desc: "参数包含 .ts 后缀 TypeScript 代码文件",
        },
        { index: 52, name: "ext_json", desc: "参数包含 .json 配置文件" },
        {
          index: 53,
          name: "ext_yaml",
          desc: "参数包含 .yaml/.yml 服务配置文件",
        },
        {
          index: 54,
          name: "ext_toml",
          desc: "参数包含 .toml 包管理器配置文件",
        },
        { index: 55, name: "ext_md", desc: "参数包含 .md 文档文件" },
        {
          index: 56,
          name: "ext_sh",
          desc: "参数包含 .sh Shell 脚本批处理文件",
        },
        { index: 57, name: "ext_txt", desc: "参数包含 .txt 纯文本文件" },
        {
          index: 58,
          name: "has_url_pattern",
          desc: "参数匹配包含 HTTP/HTTPS 等网络下载链接特征",
        },
        {
          index: 59,
          name: "has_ip_pattern",
          desc: "参数匹配包含合规格式的 IPv4 或 IPv6 网络目标地址",
        },
        {
          index: 60,
          name: "redirect_count",
          desc: "命令行重定向操作符次数统计",
        },
        { index: 61, name: "pipe_count", desc: "命令行管道连接操作符次数统计" },
        {
          index: 62,
          name: "unique_args_ratio",
          desc: "重复参数去重比，越低说明参数包含高度同质冗余内容",
        },
        {
          index: 63,
          name: "has_env_var",
          desc: "参数中涉及对系统环境变量的读取与引用 ($PATH 等)",
        },
      ],
    },
    {
      id: 2,
      name: "Instruction Embeddings",
      label: "组 C：指令语义映射向量 (Embeddings)",
      range: "64 - 95",
      dim: 32,
      color: "purple",
      description:
        "通过局部敏感哈希(LSH)在语义空间中对整条命令行进行语义降维哈希投影所得的 32 维特征向量",
      features: Array.from({ length: 32 }, (_, i) => ({
        index: 64 + i,
        name: `semantic_lsh[${i}]`,
        desc: `第 ${i + 1} 维语义 LSH 哈希投射分量。提取并哈希还原全命令行和参数的模糊语义上下文结构特征。`,
      })),
    },
    {
      id: 3,
      name: "Recent History Aggregates",
      label: "组 D：滑动窗口近期行为历史统计",
      range: "96 - 111",
      dim: 16,
      color: "orange",
      description: "记录该进程组在过去滑动窗口中的时序指标聚合",
      features: [
        {
          index: 96,
          name: "comm_match_ratio",
          desc: "相同命令在历史滑动窗口中的出现概率",
        },
        {
          index: 97,
          name: "history_block_rate",
          desc: "该进程以往触发 BLOCK (拦截)动作的比率",
        },
        {
          index: 98,
          name: "history_alert_rate",
          desc: "该进程以往触发 ALERT (告警)动作的比率",
        },
        {
          index: 99,
          name: "mean_anomaly_score",
          desc: "历史滑动窗口内平均异常判定分数",
        },
        {
          index: 100,
          name: "anomaly_variance",
          desc: "异常分数的波动差值，高方差意味着行为极度不稳定",
        },
        {
          index: 101,
          name: "category_diversity",
          desc: "历史记录中观察到的行为种类的多样性",
        },
        {
          index: 102,
          name: "anomaly_trend",
          desc: "异常值近半段与远半段的差值趋势，呈上升说明危害性可能加剧",
        },
        {
          index: 103,
          name: "time_weighted_activity",
          desc: "最近 5 次操作在该进程所属链条中的比率",
        },
        {
          index: 104,
          name: "comm_repetition_burst",
          desc: "瞬间高频重复执行相同命令的连发强度",
        },
        {
          index: 105,
          name: "comm_specific_alert_rate",
          desc: "本命令独有的历史告警概率",
        },
        {
          index: 106,
          name: "sensitive_event_ratio",
          desc: "历史事件中涉及敏感文件操作或强行终止进程的占比",
        },
        {
          index: 107,
          name: "network_event_ratio",
          desc: "历史事件中网络连接类行为的占比",
        },
        {
          index: 108,
          name: "root_event_ratio",
          desc: "历史事件以超级管理员权限启动的比例",
        },
        {
          index: 109,
          name: "distinct_comms",
          desc: "滑动窗口中去重命令名的数量 (判断命令多元程度)",
        },
        {
          index: 110,
          name: "temporal_recency",
          desc: "与上次运行该命令之间的时间长度 (1/(1+dt))",
        },
        {
          index: 111,
          name: "distinct_users",
          desc: "在滑动窗口事件中观测到的不同系统用户名总数",
        },
      ],
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
        {
          index: 112,
          name: "event_velocity",
          desc: "全局事件秒级到达流速 (反映是否处于暴力写入或扫描态)",
        },
        {
          index: 113,
          name: "distinct_pids_density",
          desc: "近期活跃进程 PID 密度，值较高通常伴随着多线程并发攻击",
        },
        {
          index: 114,
          name: "hour_of_day",
          desc: "一天中小时的归一化表示 (0 - 1.0)",
        },
        {
          index: 115,
          name: "day_of_week",
          desc: "一周中工作日的归一化表示 (0 - 1.0)",
        },
        {
          index: 116,
          name: "sin_hour",
          desc: "小时的 Sine 投射，解决跨天交界坐标轴脱节问题",
        },
        {
          index: 117,
          name: "cos_hour",
          desc: "小时的 Cosine 投射，与正弦投射结合进行二维周期表示",
        },
        {
          index: 118,
          name: "is_weekend",
          desc: "判定运行时间是否处于周六或周日 (非工作时间访问)",
        },
        {
          index: 119,
          name: "mean_historical_args_len",
          desc: "历史全部参数的平均长度聚合值",
        },
      ],
    },
    {
      id: 5,
      name: "Network Audit Features",
      label: "组 F：网络深度流审计安全标志",
      range: "120 - 127",
      dim: 8,
      color: "red",
      description:
        "由网络数据包探针提取的网络交互异常分析，映射高危害性的外发通信",
      features: [
        {
          index: 120,
          name: "net_risk_score",
          desc: "基于协议复杂度和敏感位置计算的实时网络审计评分",
        },
        {
          index: 121,
          name: "suspicious_port_flag",
          desc: "目标端口在非标应用定义之列 (例如 HTTP 采用 4444 端口)",
        },
        {
          index: 122,
          name: "reverse_shell_flag",
          desc: "通过输入输出管道建立远程交互式 Shell 强关联 (反弹 Shell 极高危特征)",
        },
        {
          index: 123,
          name: "data_exfil_flag",
          desc: "短时间、单向大字节输出特征，疑似敏感文件被批量拷贝发送",
        },
        {
          index: 124,
          name: "dns_tunnel_flag",
          desc: "检测到目的域名长度及频率极度异常，疑似走 DNS 掩护隧道",
        },
        {
          index: 125,
          name: "cleartext_proto_flag",
          desc: "检测到使用了易被嗅探的明文通信协议 (如 FTP/Telnet/Plain HTTP)",
        },
        {
          index: 126,
          name: "unusual_target_flag",
          desc: "目标主机 IP 处于地理黑名单或从未交互过的陌生外部目标",
        },
        {
          index: 127,
          name: "port_scan_flag",
          desc: "进程快速发起连续 IP 同一端口或单一 IP 多端口连接测试 (端口扫描行为)",
        },
      ],
    },
  ];
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
  { immediate: true },
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
            <div style="font-weight: 600">
              从内核探针到模型特征还原的序列化流水线
            </div>
          </template>
          <template #description>
            <div class="pipeline-desc">
              Linux 运行期间，当代理执行高危操作时，系统的
              <b>eBPF</b>
              拦截器与包装探针会捕获执行详情（程序名、参数数组、环境变量、历史活动周期、网络状态及传输元数据），并在网关侧将其还原、编码序列化为
              <b>128个平面维度浮点特征（Feature Vector）</b>。<br />
              <div v-if="isGnnModel" style="margin-top: 4px">
                在当前选中的 <b>GNN (图神经网络模型)</b> 中，这 128
                维平面特征将被<b>重塑 (Reshape) 映射至 16 个特征子图节点</b
                >，并通过 domain-specific 的图结构关系传递消息（Message
                Passing）学习高维交互作用特征。
              </div>
              <div v-else style="margin-top: 4px">
                在当前选中的 <b>{{ modelTypeLabel }}</b> 模型中，特征向量保持
                128 维平铺（Flat）格式输入分类器中，由决策森林、线性
                SVM、朴素贝叶斯或集成表决器判定安全概率。
              </div>
            </div>
          </template>
        </a-alert>
      </div>

      <!-- Tab Buttons -->
      <div class="view-toggle-bar">
        <a-radio-group
          v-model:value="activeView"
          button-style="solid"
          size="small"
        >
          <a-radio-button value="groups" v-if="isGnnModel">
            <BranchesOutlined /> 16个 GNN 交互节点重塑
          </a-radio-button>
          <a-radio-button value="dimensions">
            <SlidersOutlined /> 128维原始特征拆解
          </a-radio-button>
        </a-radio-group>
      </div>

      <!-- GNN GROUP REDIRECT INTERACTIVE DISPLAY -->
      <GnnInteractiveView v-if="activeView === 'groups' && isGnnModel" />
      <div
        v-if="activeView === 'dimensions' || !isGnnModel"
        class="flat-dimensions-container"
      >
        <a-row :gutter="[16, 16]">
          <!-- Left side: 6 major groups selection -->
          <a-col :xs="24" :md="8">
            <div class="flat-sidebar">
              <div
                v-for="group in flatGroups"
                :key="group.id"
                class="flat-group-item"
                :class="{ active: activeFlatGroupId === group.id }"
                role="button"
                tabindex="0"
                :aria-current="activeFlatGroupId === group.id"
                :aria-label="`Show feature group ${group.label}`"
                @click="activeFlatGroupId = group.id"
                @keydown.enter.prevent="activeFlatGroupId = group.id"
                @keydown.space.prevent="activeFlatGroupId = group.id"
              >
                <div class="group-header">
                  <a-tag :color="group.color" class="group-tag"
                    >Index {{ group.range }}</a-tag
                  >
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
                <a-tag
                  :color="flatGroups[activeFlatGroupId].color"
                  style="font-size: 13px; padding: 4px 8px"
                >
                  Index {{ flatGroups[activeFlatGroupId].range }}
                </a-tag>
                <span class="panel-title-text">{{
                  flatGroups[activeFlatGroupId].label
                }}</span>
                <span class="panel-subtitle-dim"
                  >包含特征数: {{ flatGroups[activeFlatGroupId].dim }}</span
                >
              </div>

              <div class="features-scroll-area">
                <table class="custom-flat-table">
                  <thead>
                    <tr>
                      <th style="width: 100px">向量位置 Index</th>
                      <th style="width: 220px">特征变量名</th>
                      <th>序列化规则与安全含义</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="feat in flatGroups[activeFlatGroupId].features"
                      :key="feat.index"
                    >
                      <td>
                        <a-tag color="default" style="font-family: monospace"
                          >F[{{ feat.index }}]</a-tag
                        >
                      </td>
                      <td>
                        <code style="color: #c41d7f">{{ feat.name }}</code>
                      </td>
                      <td style="font-size: 12px; color: #555">
                        {{ feat.desc }}
                      </td>
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

<style scoped src="./FeatureSerializationDetail.css"></style>
