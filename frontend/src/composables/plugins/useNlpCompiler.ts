import { computed, ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import type {
  VisualAction,
  VisualCondition,
  VisualConditionField,
  VisualLLMCompileResult,
  VisualLogicGroup,
  VisualLogicNode,
  VisualMapKey,
  VisualMapMode,
  VisualTrigger,
} from "../../components/plugins/types";

export interface VisualBlocksPayload {
  trigger: VisualTrigger;
  action: VisualAction;
  conditions: VisualLogicGroup;
  mapMode: VisualMapMode;
  mapKey: VisualMapKey;
  mapLimit: number;
}

export interface LlmCompileResult {
  mode: "llm" | "fallback";
  model?: string;
  reasoning?: string;
  warnings?: string[];
  error?: string;
}

const randomId = (prefix: string) => `${prefix}-${Math.random().toString(36).slice(2, 11)}`;

const parseLeafCondition = (clause: string): VisualCondition => {
  const c = clause.toLowerCase().trim();
  let field: VisualConditionField = "comm";
  let operator: VisualCondition["operator"] = "==";
  let value = "";

  if (c.includes("端口") || c.includes("port")) {
    field = "port";
  } else if (c.includes("ip") || c.includes("address") || c.includes("地址")) {
    field = "ipv4";
  } else if (c.includes("pid") || c.includes("进程号") || c.includes("进程id")) {
    field = "pid";
  } else if (c.includes("uid") || c.includes("用户id") || c.includes("用户") || c.includes("root")) {
    field = "uid";
  } else if (c.includes("gid") || c.includes("组id") || c.includes("用户组")) {
    field = "gid";
  } else if (c.includes("文件名") || c.includes("文件") || c.includes("basename")) {
    field = "basename";
  }

  if (
    c.includes("starts_with") ||
    c.includes("前缀") ||
    c.includes("以...开始") ||
    c.includes("开始于") ||
    c.includes("开头")
  ) {
    operator = "starts_with";
  } else if (
    c.includes("ends_with") ||
    c.includes("后缀") ||
    c.includes("以...结束") ||
    c.includes("结束于") ||
    c.includes("结尾")
  ) {
    operator = "ends_with";
  } else if (
    c.includes("!=") ||
    c.includes("不等于") ||
    c.includes("排除") ||
    c.includes("不是") ||
    c.includes("不匹配")
  ) {
    operator = "!=";
  }

  if (field === "uid" && (c.includes("root") || c.includes("管理员"))) {
    value = "0";
  } else if (field === "ipv4") {
    const ipMatch = clause.match(/([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)/);
    if (ipMatch) value = ipMatch[1];
  } else if (["port", "pid", "uid", "gid"].includes(field)) {
    const numMatch = clause.match(/([0-9]+)/);
    if (numMatch) value = numMatch[1];
  } else {
    const quoteMatch = clause.match(/['"“‘]([^'"”’]+)['"”’]/);
    if (quoteMatch) {
      value = quoteMatch[1];
    } else {
      const comms = [
        "python3", "busybox", "telnet", "python", "curl", "bash",
        "wget", "ssh", "ping", "perl", "ruby", "gcc", "nc", "sh",
      ];
      const found = comms.find((item) => c.includes(item));
      if (found) {
        value = found;
      } else {
        const wordMatch = clause.match(/([a-zA-Z0-9_\-.]+)/g);
        if (wordMatch) {
          const keywords = [
            "comm", "pid", "uid", "gid", "port", "ipv4", "basename",
            "starts", "ends", "or", "and", "starts_with", "ends_with",
          ];
          const validWords = wordMatch.filter(
            (word) => !keywords.includes(word.toLowerCase()) && Number.isNaN(Number(word))
          );
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
    id: randomId("cond"),
    type: "CONDITION",
    field,
    operator,
    value,
  };
};

const parseTextToGroup = (input: string): VisualLogicNode => {
  const text = input.trim();

  if (text.startsWith("(") && text.endsWith(")")) {
    let depth = 0;
    let match = true;
    for (let i = 0; i < text.length; i += 1) {
      if (text[i] === "(") depth += 1;
      else if (text[i] === ")") {
        depth -= 1;
        if (depth === 0 && i < text.length - 1) {
          match = false;
          break;
        }
      }
    }
    if (match) return parseTextToGroup(text.slice(1, -1));
  }

  let depth = 0;
  const orIndices: number[] = [];
  const andIndices: number[] = [];

  for (let i = 0; i < text.length; i += 1) {
    if (text[i] === "(") depth += 1;
    else if (text[i] === ")") depth -= 1;
    else if (depth === 0) {
      const rest = text.slice(i);
      if (
        rest.startsWith("或者") || rest.startsWith(" or ") ||
        rest.startsWith(" || ") || rest.startsWith(" 或 ")
      ) {
        orIndices.push(i);
      } else if (
        rest.startsWith("并且") || rest.startsWith(" and ") ||
        rest.startsWith(" && ") || rest.startsWith(" 且 ") ||
        rest.startsWith("，且") || rest.startsWith(",且") ||
        rest.startsWith("，") || rest.startsWith(",")
      ) {
        andIndices.push(i);
      }
    }
  }

  const buildGroup = (indices: number[], type: "AND" | "OR"): VisualLogicGroup => {
    const children: VisualLogicNode[] = [];
    let current = 0;
    for (const idx of indices) {
      children.push(parseTextToGroup(text.slice(current, idx)));
      const rest = text.slice(idx);
      if (type === "OR") {
        if (rest.startsWith("或者")) current = idx + 2;
        else if (rest.startsWith(" or ")) current = idx + 4;
        else if (rest.startsWith(" || ")) current = idx + 4;
        else if (rest.startsWith(" 或 ")) current = idx + 3;
      } else if (rest.startsWith("并且")) current = idx + 2;
      else if (rest.startsWith(" and ")) current = idx + 5;
      else if (rest.startsWith(" && ")) current = idx + 4;
      else if (rest.startsWith(" 且 ")) current = idx + 3;
      else if (rest.startsWith("，且")) current = idx + 2;
      else if (rest.startsWith(",且")) current = idx + 2;
      else if (rest.startsWith("，")) current = idx + 1;
      else if (rest.startsWith(",")) current = idx + 1;
    }
    children.push(parseTextToGroup(text.slice(current)));
    return {
      id: randomId("group"),
      type,
      children: children.filter((child) => {
        if (child.type === "CONDITION") return child.value !== "";
        return child.children.length > 0;
      }),
    };
  };

  if (orIndices.length > 0) return buildGroup(orIndices, "OR");
  if (andIndices.length > 0) return buildGroup(andIndices, "AND");

  return parseLeafCondition(text);
};

const getParsedLogicTree = (text: string): VisualLogicGroup => {
  let conditionText = text;
  const startMatch = text.match(/(?:如果|当|when|if)\s*([\s\S]+)/i);
  if (startMatch) conditionText = startMatch[1];

  const endMatch = conditionText.match(
    /([\s\S]+?)(?:时|就|，则|，将|，直接|直接|即可|，即可|就直接|就强杀|则拦截|则告警)/
  );
  if (endMatch) conditionText = endMatch[1];

  const rootNode = parseTextToGroup(conditionText);
  if (rootNode.type === "CONDITION") {
    return { id: "root", type: "AND", children: [rootNode] };
  }
  return { ...rootNode, id: "root" };
};

export const compilePromptLocally = (prompt: string): VisualBlocksPayload => {
  const p = prompt.toLowerCase().trim();
  let trigger: VisualTrigger = "process";
  let action: VisualAction = "BLOCK";
  let mapMode: VisualMapMode = "NONE";
  let mapKey: VisualMapKey = "pid";
  let mapLimit = 10;

  if (
    p.includes("socket") || p.includes("网络") || p.includes("连接") ||
    p.includes("外连") || p.includes("port") || p.includes("端口") ||
    p.includes("ip") || p.includes("外发")
  ) {
    trigger = "socket_connect";
  } else if (p.includes("设备") || p.includes("mknod") || p.includes("分区") || p.includes("节点")) {
    trigger = "inode_mknod";
  } else if (
    p.includes("内存") || p.includes("mprotect") || p.includes("执行权限") ||
    p.includes("rwx") || p.includes("shellcode")
  ) {
    trigger = "file_mprotect";
  } else if (p.includes("rename") || p.includes("重命名") || p.includes("改名") || p.includes("移动")) {
    trigger = "inode_rename";
  } else if (p.includes("unlink") || p.includes("删除") || p.includes("销毁") || p.includes("rm ")) {
    trigger = "unlink";
  } else if (p.includes("mkdir") || p.includes("创建文件夹") || p.includes("目录")) {
    trigger = "mkdir";
  } else if (p.includes("open") || p.includes("打开") || p.includes("读取")) {
    trigger = "file_open";
  }

  if (
    p.includes("kill") || p.includes("杀死") || p.includes("终结") ||
    p.includes("处死") || p.includes("强杀")
  ) {
    action = "KILL";
  } else if (
    p.includes("alert") || p.includes("告警") || p.includes("仅日志") ||
    p.includes("审计") || p.includes("静默")
  ) {
    action = "ALERT";
  }

  let conditions = getParsedLogicTree(prompt);
  if (!conditions.children || conditions.children.length === 0) {
    conditions = {
      id: "root",
      type: "AND",
      children: [{
        id: "cond-init", type: "CONDITION",
        field: "comm", operator: "==", value: "nc",
      }],
    };
  }

  if (
    p.includes("限频") || p.includes("计数") || p.includes("频率") ||
    p.includes("次数") || p.includes("counter") || p.includes("rate limit") || p.includes("累计")
  ) {
    mapMode = "COUNTER";
    const limitMatch = p.match(/(?:限制|最大|超过|阈值|threshold|次数)\s*([0-9]+)\s*(?:次)?/);
    mapLimit = limitMatch?.[1] ? parseInt(limitMatch[1], 10) : 5;
    if (p.includes("uid") || p.includes("用户")) mapKey = "uid";
    else if (p.includes("comm") || p.includes("进程名")) mapKey = "comm";
  } else if (
    p.includes("黑名单") || p.includes("黑表") || p.includes("查表") ||
    p.includes("blocklist") || p.includes("map查询") || p.includes("检索")
  ) {
    mapMode = "BLOCKLIST";
    if (p.includes("uid") || p.includes("用户")) mapKey = "uid";
    else if (p.includes("comm") || p.includes("进程名")) mapKey = "comm";
  }

  return { trigger, action, conditions, mapMode, mapKey, mapLimit };
};

export const requestLLMBlocks = async (prompt: string): Promise<VisualLLMCompileResult> => {
  const res = await axios.post<VisualLLMCompileResult>("/plugins/visual/llm-compile", { prompt });
  return res.data;
};

export function useNlpCompiler(emit: (event: "translate", payload: VisualBlocksPayload) => void) {
  const aiGenerating = ref(false);
  const lastCompileResult = ref<LlmCompileResult | null>(null);

  const lastCompileMessage = computed(() => {
    if (!lastCompileResult.value) return "";
    if (lastCompileResult.value.mode === "llm") {
      return `LLM 已生成积木流${lastCompileResult.value.model ? ` · ${lastCompileResult.value.model}` : ""}`;
    }
    return "LLM 不可用，已使用本地 NLP 规则兜底生成";
  });

  const lastCompileDescription = computed(() => {
    if (!lastCompileResult.value) return "";
    const parts = [
      lastCompileResult.value.reasoning,
      lastCompileResult.value.error ? `错误: ${lastCompileResult.value.error}` : "",
      ...(lastCompileResult.value.warnings || []),
    ].filter(Boolean);
    return parts.join("\n");
  });

  const emitBlocks = (payload: VisualBlocksPayload) => {
    emit("translate", payload);
  };

  const handleAiGenerate = async (prompt: string) => {
    if (!prompt) {
      message.warning("请输入您的安全防御指令描述！");
      return;
    }

    aiGenerating.value = true;
    try {
      const llmResult = await requestLLMBlocks(prompt);
      emitBlocks({
        trigger: llmResult.trigger,
        action: llmResult.action,
        conditions: llmResult.conditions,
        mapMode: llmResult.mapMode,
        mapKey: llmResult.mapKey,
        mapLimit: llmResult.mapLimit,
      });
      lastCompileResult.value = {
        mode: "llm",
        model: llmResult.model,
        reasoning: llmResult.reasoning,
        warnings: llmResult.warnings,
      };
      message.success("LLM 内核专家已生成积木流，已同步到工作流画布。");
    } catch (err: any) {
      const fallback = compilePromptLocally(prompt);
      emitBlocks(fallback);
      const error = err?.response?.data?.error || err?.message || "LLM 调用失败";
      lastCompileResult.value = {
        mode: "fallback",
        error,
        reasoning: "后端 LLM 不可用时，使用浏览器内置 NLP 规则保证工作台仍可继续编辑。",
      };
      message.warning(`LLM 不可用，已用本地规则兜底：${error}`);
    } finally {
      aiGenerating.value = false;
    }
  };

  return {
    aiGenerating,
    lastCompileResult,
    lastCompileMessage,
    lastCompileDescription,
    handleAiGenerate,
  };
}
