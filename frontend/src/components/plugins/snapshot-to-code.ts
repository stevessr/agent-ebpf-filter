import type { VisualLogicNode, VisualWorkspaceSnapshot } from "./types";

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

  let pseudo = `import { ${trigger}, Action, Maps, HookContext } from "ebpf";

export default function filter(ctx: HookContext) {
  // 1. 获取内核 Hook 上下文变量
  const comm = ctx.comm;
  const pid = ctx.pid;
  const uid = ctx.uid;
  const basename = ctx.basename;
  const port = ctx.port;
  const ipv4 = ctx.ipv4;
  const gid = ctx.gid;
  const ppid = ctx.ppid;
  const loginuid = ctx.loginuid;

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
