<script setup lang="ts">
import { computed, ref } from "vue";

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

const hoveredNodeId = ref<number | null>(null);
// source, target indices (matching gnnNodes id)
const gnnStaticEdges = [
  [0, 1],
  [1, 2],
  [1, 3],
  [2, 5],
  [2, 6],
  [2, 7],
  [2, 14],
  [3, 4],
  [4, 5],
  [4, 7],
  [8, 9],
  [10, 11],
  [11, 12],
  [10, 12],
  [13, 14],
  [5, 13],
  [7, 14],
  [0, 13],
  [1, 13],
  [0, 14],
];

const gnnNodes: GnnNode[] = [
  {
    id: 0,
    name: "behavior_category",
    label: "行为类别独热码",
    start: 0,
    end: 15,
    dim: 15,
    description: "追踪事件的行为分类独热编码",
    details:
      "包括未识别(UNKNOWN=0)、文件读取(FILE_READ=1)、文件写入(FILE_WRITE=2)、文件删除(FILE_DELETE=3)、文件权限(FILE_PERMISSION=4)、网络通信(NETWORK=5)、进程启动(PROCESS_EXEC=6)、进程杀死(PROCESS_KILL=7)、系统元数据(SYSTEM_INFO=8)、包管理器(PACKAGE_MANAGER=9)、数据库操作(DATABASE=10)、压缩归档(COMPRESSION=11)、开发构建(DEVELOPMENT=12)、容器编排(CONTAINER=13)、敏感提权(SENSITIVE=14)等 15 维一热特征表示。",
  },
  {
    id: 1,
    name: "process_flags",
    label: "进程属性标志位",
    start: 15,
    end: 22,
    dim: 7,
    description: "当前执行进程的身份属性标记",
    details:
      "指示是否为 Shell 终端(is_shell)、包管理器(is_package_manager)、系统 Agent CLI(is_agent_cli)、以及是否以 root 权限运行(is_root_user)、命令行是否携带网络相关参数(has_network_args)、路径相关参数(has_file_args)等特征。",
  },
  {
    id: 2,
    name: "io_flags",
    label: "I/O 结构特征",
    start: 22,
    end: 28,
    dim: 6,
    description: "输入输出重定向及管道链特征",
    details:
      "编码命令行中是否包含标准输出重定向(>)、追加重定向(>>)、输入重定向(<)、管道链操作(|)、以及是否存在设备文件读写(/dev/)等 I/O 标志。",
  },
  {
    id: 3,
    name: "command_meta",
    label: "命令元数据",
    start: 28,
    end: 32,
    dim: 4,
    description: "命令名称长度与参数总量统计",
    details:
      "包含命令名字符长度、命令行参数(args)个数等经过归一化限制在[0, 1]区间的连续数值特征。",
  },
  {
    id: 4,
    name: "arg_statistics",
    label: "参数统计特征",
    start: 32,
    end: 38,
    dim: 6,
    description: "命令行参数的长度分布与熵值",
    details:
      "包含参数平均长度、参数长度标准差、总参数字节数、以及命令行字符的香农熵(Shannon Entropy)以辅助评估是否包含加密或混淆内容。",
  },
  {
    id: 5,
    name: "sensitive_paths",
    label: "敏感路径命率",
    start: 38,
    end: 48,
    dim: 10,
    description: "对系统敏感关键目录的访问标记",
    details:
      "包含命令中是否涉及 /etc/, /proc/, /sys/, /var/log/, /root/, /home/, /tmp/, ~/.ssh, ~/.gnupg, /boot/ 等 10 类系统敏感路径的包含状态。",
  },
  {
    id: 6,
    name: "file_extensions",
    label: "文件后缀直方图",
    start: 48,
    end: 58,
    dim: 10,
    description: "命中特定类型文件后缀的统计",
    details:
      "统计命令参数中以 10 种常见后缀(.go, .py, .js, .ts, .json, .yaml, .toml, .md, .sh, .txt)结尾的文件命中频次及占比分布。",
  },
  {
    id: 7,
    name: "url_patterns",
    label: "网络模式匹配",
    start: 58,
    end: 64,
    dim: 6,
    description: "参数中含有 IP 地址或 URL 的指示器",
    details:
      "判定命令行中是否包含 HTTP/HTTPS 链接、正则匹配的 IPv4/IPv6 地址、重定向操作计数、以及环境变量引用检测等。",
  },
  {
    id: 8,
    name: "embedding_low",
    label: "语义词嵌入(低阶)",
    start: 64,
    end: 80,
    dim: 16,
    description: "LSH 指令语义编码的低阶维度分量",
    details:
      "对命令行全长文本进行语义提取(Instruction Embedding)，通过局部敏感哈希(LSH)降维得到的 64 维特征中的前 16 维分量。",
  },
  {
    id: 9,
    name: "embedding_high",
    label: "语义词嵌入(高阶)",
    start: 80,
    end: 96,
    dim: 16,
    description: "LSH 指令语义编码的高阶维度分量",
    details:
      "局部敏感哈希(LSH)指令语义特征物化降维向量中第 17 至 32 维，与低维分量联合表示操作的高维上下文语义。",
  },
  {
    id: 10,
    name: "freq_history",
    label: "历史频率指标",
    start: 96,
    end: 104,
    dim: 8,
    description: "滑动窗口中该命令及状态的频次比率",
    details:
      "包含该命令名在最近 100 次历史事件中的出现频次、历史拦截率(BLOCK)、历史报警率(ALERT)、以及历史检测异常分数的平均值与方差。",
  },
  {
    id: 11,
    name: "pattern_history",
    label: "历史模式指标",
    start: 104,
    end: 112,
    dim: 8,
    description: "历史窗口动作多样性与时间紧凑性",
    details:
      "包含历史异常分数变化趋势、命令重复爆发强度、特定命令告警率、去重敏感事件占比以及历史记录涉及的用户数与命令回溯时间间距。",
  },
  {
    id: 12,
    name: "temporal",
    label: "实时时序特征",
    start: 112,
    end: 120,
    dim: 8,
    description: "事件产生的流速、PID 密度及时间周期",
    details:
      "包含每秒捕获事件流速、当前并发 PID 数量、一天中小时的归一化和正余弦周期编码、是否为周末等时序特征。",
  },
  {
    id: 13,
    name: "risk_scores",
    label: "网络风险打分",
    start: 120,
    end: 124,
    dim: 4,
    description: "静态网络风险评判和协议安全分析",
    details:
      "结合网络抓包上下文进行网络行为审计，综合分析协议、目的 IP、数据量大小得出的基础网络威胁等级特征。",
  },
  {
    id: 14,
    name: "network_anomalies",
    label: "网络异常标志位",
    start: 124,
    end: 128,
    dim: 4,
    description: "是否触发网络高危连接规则",
    details:
      "指示是否涉及非常规高危端口、潜在反弹 Shell 连接(Reverse Shell)、大流量数据外泄(Data Exfiltration)、以及 DNS 隧道异常等严重安全特征。",
  },
  {
    id: 15,
    name: "global_summary",
    label: "全局概况汇总节点",
    start: 0,
    end: 128,
    dim: 128,
    description: "全维特征线性映射的汇总交互池",
    details:
      "由 128 维特征整体投影生成的超级聚合节点(Super Node)，在 GNN 每一层消息传递中接收并广播到全网，担当全局记忆库。",
  },
];

const isGnnConnected = (nodeId1: number | null, nodeId2: number): boolean => {
  if (nodeId1 === null || nodeId1 === nodeId2) return true;
  // Node 15 (global_summary) connects to all nodes (0-14)
  if (nodeId1 === 15 || nodeId2 === 15) return true;
  return gnnStaticEdges.some(
    (e) =>
      (e[0] === nodeId1 && e[1] === nodeId2) ||
      (e[0] === nodeId2 && e[1] === nodeId1),
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

const selectedNodeDetails = computed<GnnNode | null>(() => {
  if (hoveredNodeId.value === null) return null;
  return gnnNodes.find((n) => n.id === hoveredNodeId.value) || null;
});
</script>

<template>
  <div class="gnn-interactive-container">
    <a-row :gutter="[16, 16]">
      <!-- Grid of GNN Nodes -->
      <a-col :xs="24" :lg="16">
        <div class="gnn-title">
          <span
            ><b>GNN 特征重塑节点拓扑</b>
            (鼠标悬停某一节点以查看其信息传递通路)</span
          >
        </div>

        <div class="gnn-grid">
          <div
            v-for="node in gnnNodes"
            :key="node.id"
            class="gnn-node-pill"
            :class="{
              active: hoveredNodeId === node.id,
              connected:
                hoveredNodeId !== null &&
                hoveredNodeId !== node.id &&
                isGnnConnected(hoveredNodeId, node.id),
              dimmed:
                hoveredNodeId !== null &&
                hoveredNodeId !== node.id &&
                !isGnnConnected(hoveredNodeId, node.id),
              'global-super-node': node.id === 15,
            }"
            @mouseenter="hoveredNodeId = node.id"
            @mouseleave="hoveredNodeId = null"
          >
            <div class="node-id">#{{ node.id }}</div>
            <div class="node-name">{{ node.name }}</div>
            <div class="node-label">{{ node.label }}</div>
            <div class="node-badge-row">
              <span class="badge range"
                >F[{{ node.start }}-{{ node.end - 1 }}]</span
              >
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
              <span class="panel-node-id"
                >Node #{{ selectedNodeDetails.id }}</span
              >
              <h4 class="panel-node-title">
                {{ selectedNodeDetails.label }}
              </h4>
              <code class="panel-node-code">{{
                selectedNodeDetails.name
              }}</code>
            </div>

            <div class="panel-section">
              <div class="section-title">原始特征片段索引</div>
              <div class="index-box">
                <span
                  >区间索引:
                  <b
                    >{{ selectedNodeDetails.start }} -
                    {{ selectedNodeDetails.end - 1 }}</b
                  ></span
                >
                <span style="margin-left: 16px"
                  >所含维度: <b>{{ selectedNodeDetails.dim }} 维</b></span
                >
              </div>
            </div>

            <div class="panel-section">
              <div class="section-title">功能简述</div>
              <p class="section-text">
                {{ selectedNodeDetails.description }}
              </p>
            </div>

            <div class="panel-section">
              <div class="section-title">序列化细节</div>
              <p class="section-text details-text">
                {{ selectedNodeDetails.details }}
              </p>
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
                  :class="{ super: cid === 15 }"
                  @mouseenter="hoveredNodeId = cid"
                >
                  #{{ cid }} {{ gnnNodes[cid].name }}
                </div>
              </div>
            </div>
          </template>
          <div v-else class="empty-panel">
            <InfoCircleOutlined class="empty-icon" />
            <p>
              将鼠标指针悬停左侧 GNN
              节点卡片上，即可动态分析其特征在消息传递阶段的网络连接通路与序列化内容。
            </p>
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
            <td>
              <a-tag :color="node.id === 15 ? 'purple' : 'blue'"
                >#{{ node.id }}</a-tag
              >
            </td>
            <td>
              <code>{{ node.name }}</code>
            </td>
            <td>
              <b>{{ node.label }}</b>
            </td>
            <td>
              <code>F[{{ node.start }} - {{ node.end - 1 }}]</code>
            </td>
            <td>{{ node.dim }}</td>
            <td style="font-size: 12px; color: #4a4a4a">
              {{ node.details }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
