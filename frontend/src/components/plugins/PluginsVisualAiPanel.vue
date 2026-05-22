<script setup lang="ts">
import { ref, watch } from "vue";
import { message } from "ant-design-vue";
import { ThunderboltOutlined } from "@ant-design/icons-vue";
import type { VisualLogicNode, VisualLogicGroup, VisualCondition } from "./types";

const props = defineProps<{
  modelValue: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", val: string): void;
  (
    e: "translate",
    payload: {
      trigger: "process" | "file_open" | "mkdir" | "file_create" | "rmdir" | "symlink" | "unlink" | "socket_connect" | "inode_mknod" | "file_mprotect" | "inode_rename";
      action: "BLOCK" | "ALERT" | "KILL";
      conditions: VisualLogicGroup;
      mapMode: "NONE" | "COUNTER" | "BLOCKLIST";
      mapKey: "uid" | "pid" | "comm";
      mapLimit: number;
    }
  ): void;
}>();

const localPrompt = ref(props.modelValue);
watch(
  () => props.modelValue,
  (val) => {
    localPrompt.value = val;
  }
);
watch(localPrompt, (val) => {
  emit("update:modelValue", val);
});

const aiGenerating = ref(false);

const applyExample = (text: string) => {
  localPrompt.value = text;
};

// Recursive Logic Parsing Helpers
const parseLeafCondition = (clause: string): VisualCondition => {
  const c = clause.toLowerCase().trim();
  let field: "comm" | "pid" | "uid" | "basename" | "port" | "ipv4" | "gid" = "comm";
  let operator: "==" | "!=" | "starts_with" | "ends_with" = "==";
  let value = "";

  // 1. Detect field
  if (c.includes("端口") || c.includes("port")) {
    field = "port";
  } else if (c.includes("ip") || c.includes("address") || c.includes("地址")) {
    field = "ipv4";
  } else if (c.includes("pid") || c.includes("进程号") || c.includes("进程id")) {
    field = "pid";
  } else if (c.includes("uid") || c.includes("用户id") || c.includes("用户") || c.includes("root") || c.includes("uid")) {
    field = "uid";
  } else if (c.includes("gid") || c.includes("组id") || c.includes("用户组") || c.includes("gid")) {
    field = "gid";
  } else if (c.includes("文件名") || c.includes("文件") || c.includes("basename")) {
    field = "basename";
  } else {
    field = "comm";
  }

  // 2. Detect operator
  if (c.includes("starts_with") || c.includes("前缀") || c.includes("以...开始") || c.includes("开始于") || c.includes("开头")) {
    operator = "starts_with";
  } else if (c.includes("ends_with") || c.includes("后缀") || c.includes("以...结束") || c.includes("结束于") || c.includes("结尾")) {
    operator = "ends_with";
  } else if (c.includes("!=") || c.includes("不等于") || c.includes("排除") || c.includes("不是") || c.includes("不匹配")) {
    operator = "!=";
  } else {
    operator = "==";
  }

  // 3. Extract value
  if (field === "uid" && (c.includes("root") || c.includes("管理员"))) {
    value = "0";
  } else if (field === "ipv4") {
    const ipMatch = clause.match(/([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)/);
    if (ipMatch) value = ipMatch[1];
  } else if (field === "port" || field === "pid" || field === "uid" || field === "gid") {
    const numMatch = clause.match(/([0-9]+)/);
    if (numMatch) value = numMatch[1];
  } else {
    // Check quotes first
    const quoteMatch = clause.match(/['"“‘]([^'"“”’]+)['"”’]/);
    if (quoteMatch) {
      value = quoteMatch[1];
    } else {
      const comms = ["nc", "curl", "python", "bash", "wget", "ssh", "ping", "python3", "perl", "ruby", "gcc", "sh", "busybox", "telnet"];
      let found = "";
      for (const item of comms) {
        if (c.includes(item)) {
          found = item;
          break;
        }
      }
      if (found) {
        value = found;
      } else {
        const wordMatch = clause.match(/([a-zA-Z0-9_\-\.]+)/g);
        if (wordMatch) {
          const keywords = ["comm", "pid", "uid", "gid", "port", "ipv4", "basename", "starts", "ends", "or", "and", "starts_with", "ends_with"];
          const validWords = wordMatch.filter(w => !keywords.includes(w.toLowerCase()) && isNaN(Number(w)));
          if (validWords.length > 0) {
            value = validWords[validWords.length - 1];
          }
        }
      }
    }
  }

  if (!value) {
    if (field === "comm") value = "nc";
    else if (field === "pid") value = "1234";
    else if (field === "uid") value = "0";
    else if (field === "gid") value = "0";
    else if (field === "port") value = "4444";
    else if (field === "ipv4") value = "127.0.0.1";
    else value = "file";
  }

  return {
    id: `cond-${Math.random().toString(36).substr(2, 9)}`,
    type: "CONDITION",
    field,
    operator,
    value,
  };
};

const parseTextToGroup = (text: string): VisualLogicNode => {
  text = text.trim();
  
  if (text.startsWith("(") && text.endsWith(")")) {
    let depth = 0;
    let match = true;
    for (let i = 0; i < text.length; i++) {
      if (text[i] === "(") depth++;
      else if (text[i] === ")") {
        depth--;
        if (depth === 0 && i < text.length - 1) {
          match = false;
          break;
        }
      }
    }
    if (match) {
      return parseTextToGroup(text.substring(1, text.length - 1));
    }
  }

  let depth = 0;
  const orIndices: number[] = [];
  const andIndices: number[] = [];

  for (let i = 0; i < text.length; i++) {
    if (text[i] === "(") {
      depth++;
    } else if (text[i] === ")") {
      depth--;
    } else if (depth === 0) {
      const rest = text.substring(i);
      if (rest.startsWith("或者") || rest.startsWith(" or ") || rest.startsWith(" || ") || rest.startsWith(" 或 ")) {
        orIndices.push(i);
      }
      else if (rest.startsWith("并且") || rest.startsWith(" and ") || rest.startsWith(" && ") || rest.startsWith(" 且 ") || rest.startsWith("，且") || rest.startsWith(",且") || rest.startsWith("，") || rest.startsWith(",")) {
        andIndices.push(i);
      }
    }
  }

  if (orIndices.length > 0) {
    const children: VisualLogicNode[] = [];
    let current = 0;
    for (const idx of orIndices) {
      const part = text.substring(current, idx);
      children.push(parseTextToGroup(part));
      
      const rest = text.substring(idx);
      if (rest.startsWith("或者")) current = idx + 2;
      else if (rest.startsWith(" or ")) current = idx + 4;
      else if (rest.startsWith(" || ")) current = idx + 4;
      else if (rest.startsWith(" 或 ")) current = idx + 3;
    }
    children.push(parseTextToGroup(text.substring(current)));
    return {
      id: `group-${Math.random().toString(36).substr(2, 9)}`,
      type: "OR",
      children: children.filter(c => {
        if (!c) return false;
        if (c.type === "CONDITION") return c.value !== "";
        return c.children && c.children.length > 0;
      }),
    };
  }

  if (andIndices.length > 0) {
    const children: VisualLogicNode[] = [];
    let current = 0;
    for (const idx of andIndices) {
      const part = text.substring(current, idx);
      children.push(parseTextToGroup(part));
      
      const rest = text.substring(idx);
      if (rest.startsWith("并且")) current = idx + 2;
      else if (rest.startsWith(" and ")) current = idx + 5;
      else if (rest.startsWith(" && ")) current = idx + 4;
      else if (rest.startsWith(" 且 ")) current = idx + 3;
      else if (rest.startsWith("，且")) current = idx + 2;
      else if (rest.startsWith(",且")) current = idx + 2;
      else if (rest.startsWith("，")) current = idx + 1;
      else if (rest.startsWith(",")) current = idx + 1;
    }
    children.push(parseTextToGroup(text.substring(current)));
    return {
      id: `group-${Math.random().toString(36).substr(2, 9)}`,
      type: "AND",
      children: children.filter(c => {
        if (!c) return false;
        if (c.type === "CONDITION") return c.value !== "";
        return c.children && c.children.length > 0;
      }),
    };
  }

  return parseLeafCondition(text);
};

const getParsedLogicTree = (text: string): VisualLogicGroup => {
  let conditionText = text;
  const startMatch = text.match(/(?:如果|当|when|if)\s*([\s\S]+)/i);
  if (startMatch) {
    conditionText = startMatch[1];
  }
  
  const endMatch = conditionText.match(/([\s\S]+?)(?:时|就|，则|，将|，直接|直接|即可|，即可|就直接|就强杀|则拦截|则告警)/);
  if (endMatch) {
    conditionText = endMatch[1];
  }

  const rootNode = parseTextToGroup(conditionText);
  if (rootNode.type === "CONDITION") {
    return {
      id: "root",
      type: "AND",
      children: [rootNode],
    };
  } else {
    return {
      ...rootNode,
      id: "root",
    } as VisualLogicGroup;
  }
};

// NLP Heuristic natural language compile translator
const handleAiGenerate = () => {
  const p = localPrompt.value.toLowerCase().trim();
  if (!p) {
    message.warning("请输入您的安全防御指令描述！");
    return;
  }

  aiGenerating.value = true;
  try {
    let conditions: VisualLogicGroup;
    let trigger: "process" | "file_open" | "mkdir" | "file_create" | "rmdir" | "symlink" | "unlink" | "socket_connect" | "inode_mknod" | "file_mprotect" | "inode_rename" = "process";
    let action: "BLOCK" | "ALERT" | "KILL" = "BLOCK";
    let mapMode: "NONE" | "COUNTER" | "BLOCKLIST" = "NONE";
    let mapKey: "uid" | "pid" | "comm" = "pid";
    let mapLimit = 10;

    // 1. Detect Trigger Hook
    if (
      p.includes("socket") ||
      p.includes("网络") ||
      p.includes("连接") ||
      p.includes("外连") ||
      p.includes("port") ||
      p.includes("端口") ||
      p.includes("ip") ||
      p.includes("外发")
    ) {
      trigger = "socket_connect";
    } else if (
      p.includes("设备") ||
      p.includes("mknod") ||
      p.includes("分区") ||
      p.includes("节点")
    ) {
      trigger = "inode_mknod";
    } else if (
      p.includes("内存") ||
      p.includes("mprotect") ||
      p.includes("执行权限") ||
      p.includes("rwx") ||
      p.includes("shellcode")
    ) {
      trigger = "file_mprotect";
    } else if (
      p.includes("rename") ||
      p.includes("重命名") ||
      p.includes("改名") ||
      p.includes("移动")
    ) {
      trigger = "inode_rename";
    } else if (
      p.includes("unlink") ||
      p.includes("删除") ||
      p.includes("销毁") ||
      p.includes("rm ")
    ) {
      trigger = "unlink";
    } else if (
      p.includes("mkdir") ||
      p.includes("创建文件夹") ||
      p.includes("目录")
    ) {
      trigger = "mkdir";
    } else if (
      p.includes("open") ||
      p.includes("打开") ||
      p.includes("读取")
    ) {
      trigger = "file_open";
    }

    // 2. Detect Action Hook
    if (
      p.includes("kill") ||
      p.includes("杀死") ||
      p.includes("终结") ||
      p.includes("处死") ||
      p.includes("强杀")
    ) {
      action = "KILL";
    } else if (
      p.includes("alert") ||
      p.includes("告警") ||
      p.includes("仅日志") ||
      p.includes("审计") ||
      p.includes("静默")
    ) {
      action = "ALERT";
    }

    // 3. Extract conditions matchers
    conditions = getParsedLogicTree(localPrompt.value);

    if (!conditions.children || conditions.children.length === 0) {
      conditions = {
        id: "root",
        type: "AND",
        children: [{
          id: "cond-init",
          type: "CONDITION",
          field: "comm",
          operator: "==",
          value: "nc",
        }],
      };
    }

    // 4. Map stateful operation parsing
    if (
      p.includes("限频") ||
      p.includes("计数") ||
      p.includes("频率") ||
      p.includes("次数") ||
      p.includes("counter") ||
      p.includes("rate limit") ||
      p.includes("累计")
    ) {
      mapMode = "COUNTER";
      const limitMatch = p.match(
        /(?:限制|最大|超过|阈值|threshold|次数)\s*([0-9]+)\s*(?:次)?/
      );
      if (limitMatch && limitMatch[1]) {
        mapLimit = parseInt(limitMatch[1], 10);
      } else {
        mapLimit = 5;
      }

      if (p.includes("uid") || p.includes("用户")) {
        mapKey = "uid";
      } else if (p.includes("comm") || p.includes("进程名")) {
        mapKey = "comm";
      } else {
        mapKey = "pid";
      }
    } else if (
      p.includes("黑名单") ||
      p.includes("黑表") ||
      p.includes("查表") ||
      p.includes("blocklist") ||
      p.includes("map查询") ||
      p.includes("检索")
    ) {
      mapMode = "BLOCKLIST";
      if (p.includes("uid") || p.includes("用户")) {
        mapKey = "uid";
      } else if (p.includes("comm") || p.includes("进程名")) {
        mapKey = "comm";
      } else {
        mapKey = "pid";
      }
    }

    emit("translate", {
      trigger,
      action,
      conditions,
      mapMode,
      mapKey,
      mapLimit,
    });
    message.success("AI 内核专家智能规则拼装成功！积木块参数已自动配齐。");
  } catch (err: any) {
    message.error("智能转译失败: " + err.message);
  } finally {
    aiGenerating.value = false;
  }
};
</script>

<template>
  <div class="block-card ai-copilot-card">
    <div class="block-header">
      <span class="block-badge">AI Copilot</span>
      <strong style="color: #fff">AI 智能内核防御助手 (NLP Blocks Compiler)</strong>
    </div>
    <div class="block-body">
      <div class="desc-line">
        用自然语言描述您的主动防御拦截意图，AI 助手将自动帮您拼装整条积木流：
      </div>
      <a-textarea
        v-model:value="localPrompt"
        placeholder="例如：当有人使用 python 运行网络连接，且外连端口为 4444 时，直接强杀该进程，并启用计数器限制其最大触发频率为 3 次。"
        :rows="3"
        class="ai-textarea"
      />
      <div class="control-footer">
        <div class="ai-prompts-examples">
          快捷指令示例：
          <a-tag @click="applyExample('阻止 nc 进程运行，并且直接杀死进程')" class="example-tag">阻断并杀死nc</a-tag>
          <a-tag @click="applyExample('当外连端口为 4444 时强杀进程，并限频计数最多 5 次')" class="example-tag">外连4444强杀限频5次</a-tag>
          <a-tag @click="applyExample('拦截对 shadow 文件的重命名操作并发出警告')" class="example-tag">勒索shadow重命名保护</a-tag>
        </div>
        <a-button type="primary" :loading="aiGenerating" @click="handleAiGenerate" class="ai-btn">
          <template #icon><ThunderboltOutlined /></template>
          AI 智能积木生成
        </a-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Blueprint nodes styling */
.block-card {
  border-radius: 8px;
  overflow: visible;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  background: rgba(13, 19, 33, 0.85);
  backdrop-filter: blur(8px);
  transition: all 0.3s ease;
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.block-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.7);
}

.ai-copilot-card {
  margin-bottom: 20px;
  border-color: rgba(114, 46, 209, 0.35) !important;
}
.ai-copilot-card:hover {
  border-color: rgba(114, 46, 209, 0.7) !important;
  box-shadow: 0 0 15px rgba(114, 46, 209, 0.2);
}

.block-header {
  padding: 10px 14px;
  display: flex;
  align-items: center;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  background: linear-gradient(135deg, #722ed1, #39006a);
}
.block-badge {
  background: rgba(0, 0, 0, 0.35);
  color: white;
  padding: 2px 8px;
  font-size: 11px;
  border-radius: 4px;
  margin-right: 12px;
  font-weight: bold;
}
.block-body {
  background: #0f172a;
  padding: 18px;
  color: #cbd5e1;
}
.desc-line {
  font-size: 13px;
  color: #b494db;
  font-weight: 500;
  margin-bottom: 12px;
}

.control-footer {
  margin-top: 12px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.ai-prompts-examples {
  font-size: 11px;
  color: #94a3b8;
}

.example-tag {
  cursor: pointer;
  margin-right: 4px;
  background-color: rgba(114, 46, 209, 0.15) !important;
  border-color: rgba(114, 46, 209, 0.3) !important;
  color: #d3adf7 !important;
  transition: all 0.2s ease;
}
.example-tag:hover {
  background-color: rgba(114, 46, 209, 0.3) !important;
  border-color: rgba(114, 46, 209, 0.6) !important;
  color: #e9d5ff !important;
}

.ai-btn {
  background: #722ed1 !important;
  border-color: #722ed1 !important;
  transition: all 0.3s ease;
}
.ai-btn:hover {
  background: #85a5ff !important; /* Ant default hover or similar */
  box-shadow: 0 0 8px rgba(114, 46, 209, 0.6);
}

/* Deep input styling for dark mode */
:deep(.ai-textarea) {
  background-color: #1e293b !important;
  border-color: #334155 !important;
  color: #f1f5f9 !important;
  border-radius: 6px;
}
:deep(.ai-textarea::placeholder) {
  color: #475569 !important;
}
:deep(.ai-textarea:focus) {
  border-color: #722ed1 !important;
  box-shadow: 0 0 0 2px rgba(114, 46, 209, 0.2) !important;
}
</style>

