import type {
  VisualTrigger,
  VisualAction,
  VisualMapKey,
  VisualLogicGroup,
  VisualLogicNode,
  VisualCondition,
  VisualWorkspaceSnapshot,
} from "./types";

/**
 * 将 VisualWorkspaceSnapshot (积木状态) 反向生成 TS 风格的伪代码
 */
export const snapshotToPseudoCode = (
  snapshot: VisualWorkspaceSnapshot
): string => {
  const { trigger, action, conditions, mapMode, mapKey, mapLimit } = snapshot;

  const renderConditions = (node: VisualLogicNode, indent = "  "): string => {
    if (node.type === "CONDITION") {
      const val = node.value.trim();
      const quote =
        isNaN(Number(val)) && val !== "true" && val !== "false"
          ? `"${val}"`
          : val;
      return `ctx.${node.field} ${
        node.operator === "=="
          ? "==="
          : node.operator === "!="
          ? "!=="
          : node.operator
      } ${quote}`;
    } else {
      if (!node.children || node.children.length === 0) return "true";
      const op = node.type === "AND" ? " && " : " || ";
      const exprs = node.children.map((child) => {
        const sub = renderConditions(child, indent);
        return child.type === "CONDITION" ? sub : `(${sub})`;
      });
      return exprs.join(op);
    }
  };

  const conditionExpr = renderConditions(conditions);

  let pseudo = `import { ${trigger}, Action, Maps } from "ebpf";

export default function filter(ctx: any) {
  // 1. 获取内核 Hook 上下文变量
  const comm = ctx.comm;
  const pid = ctx.pid;
  const uid = ctx.uid;
  const basename = ctx.basename;
  const port = ctx.port;
  const ipv4 = ctx.ipv4;
  const gid = ctx.gid;

  // 2. 嵌套逻辑匹配
  if (${conditionExpr}) {
`;

  if (mapMode === "COUNTER") {
    pseudo += `    // 3. 状态 Map 计数限频
    const rateLimit = Maps.createCounter({ key: "${mapKey}", limit: ${mapLimit} });
    if (rateLimit.exceeded()) {
      // 4. 执行响应动作
      Action.${action.toLowerCase()}();
    }
`;
  } else if (mapMode === "BLOCKLIST") {
    pseudo += `    // 3. 状态 Map 查黑名单
    const blocklist = Maps.createBlocklist({ key: "${mapKey}" });
    if (blocklist.matched()) {
      // 4. 执行响应动作
      Action.${action.toLowerCase()}();
    }
`;
  } else {
    pseudo += `    // 3. 直接执行响应动作
    Action.${action.toLowerCase()}();
`;
  }

  pseudo += `  }
}
`;
  return pseudo;
};

/**
 * 将 TS 风格伪代码编译/解析为 VisualWorkspaceSnapshot (积木工作台状态)
 */
export const pseudoCodeToSnapshot = (
  code: string,
  existingSnapshot: VisualWorkspaceSnapshot
): VisualWorkspaceSnapshot => {
  const result = { ...existingSnapshot };

  // 1. 解析 trigger
  const importMatch = code.match(/import\s*\{\s*(\w+)/);
  if (importMatch && importMatch[1]) {
    const parsedTrigger = importMatch[1].trim() as VisualTrigger;
    const allTriggers = [
      "process",
      "file_open",
      "mkdir",
      "file_create",
      "rmdir",
      "symlink",
      "unlink",
      "socket_connect",
      "inode_mknod",
      "file_mprotect",
      "inode_rename",
    ];
    if (allTriggers.includes(parsedTrigger)) {
      result.trigger = parsedTrigger;
    }
  }

  // 2. 解析 Action
  const actionMatch = code.match(/Action\.(\w+)\(\)/i);
  if (actionMatch && actionMatch[1]) {
    const act = actionMatch[1].toUpperCase() as VisualAction;
    if (["BLOCK", "ALERT", "KILL"].includes(act)) {
      result.action = act;
    }
  }

  // 3. 解析 MapMode & MapKey & MapLimit
  if (code.includes("Maps.createCounter")) {
    result.mapMode = "COUNTER";
    const keyMatch = code.match(/Maps\.createCounter\(\{\s*key:\s*"(\w+)"/);
    if (keyMatch && keyMatch[1]) result.mapKey = keyMatch[1] as VisualMapKey;
    const limitMatch = code.match(/limit:\s*(\d+)/);
    if (limitMatch && limitMatch[1])
      result.mapLimit = parseInt(limitMatch[1], 10);
  } else if (code.includes("Maps.createBlocklist")) {
    result.mapMode = "BLOCKLIST";
    const keyMatch = code.match(/Maps\.createBlocklist\(\{\s*key:\s*"(\w+)"/);
    if (keyMatch && keyMatch[1]) result.mapKey = keyMatch[1] as VisualMapKey;
  } else {
    result.mapMode = "NONE";
  }

  // 4. 解析 if 条件部分 (简单 AST-like 词法还原条件树)
  const ifMatch = code.match(/if\s*\(([\s\S]*?)\)\s*\{/);
  if (ifMatch && ifMatch[1]) {
    let condStr = ifMatch[1].trim();
    // 移除多余换行、空格
    condStr = condStr.replace(/\s+/g, " ");

    // 简单解析器，支持 `ctx.field === "val"` 组合
    const parseSimpleExpression = (expr: string): VisualLogicGroup => {
      // 简单起见，按 && / || 分割
      // 这里支持最基础的一层 AND/OR，若需要深层嵌套则进行递归分词
      const hasOr = expr.includes("||");
      const delimiter = hasOr ? "||" : "&&";
      const parts = expr.split(delimiter);

      const children: Array<VisualLogicGroup | VisualCondition> = [];

      parts.forEach((part, idx) => {
        const trimmed = part
          .trim()
          .replace(/^\(|\)$/g, "")
          .trim();
        // 匹配 ctx.field === "val" 或 ctx.field !== 123
        const condMatch = trimmed.match(
          /ctx\.(\w+)\s*(===|!==|==|!=|starts_with|ends_with)\s*(["']?)(.*?)\3/
        );
        if (condMatch) {
          const field = condMatch[1];
          const opRaw = condMatch[2];
          const val = condMatch[4];

          const operator =
            opRaw === "===" || opRaw === "=="
              ? "=="
              : opRaw === "!==" || opRaw === "!="
              ? "!="
              : (opRaw as any);

          children.push({
            id: `cond-ts-${idx}-${Math.random().toString(36).substr(2, 5)}`,
            type: "CONDITION",
            field: field as any,
            operator,
            value: val,
          });
        }
      });

      return {
        id: "root",
        type: hasOr ? "OR" : "AND",
        children,
      };
    };

    try {
      result.conditions = parseSimpleExpression(condStr);
    } catch (e) {
      console.warn("Failed to parse pseudo-code conditions tree:", e);
    }
  }

  return result;
};
