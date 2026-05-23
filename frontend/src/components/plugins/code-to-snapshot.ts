import * as acorn from "acorn";
import type {
  VisualTrigger,
  VisualAction,
  VisualMapKey,
  VisualLogicGroup,
  VisualCondition,
  VisualWorkspaceSnapshot,
} from "./types";

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

  // 4. 解析 if 条件部分 (采用 Acorn 解析标准 JS/TS 表达式 AST，并递归转为工作台结构)
  const ifMatch = code.match(/if\s*\(([\s\S]*?)\)\s*\{/);
  if (ifMatch && ifMatch[1]) {
    const condStr = ifMatch[1].trim();

    // 递归将 Acorn AST 转换为 VisualLogicNode
    const walkAst = (node: any): VisualLogicGroup | VisualCondition => {
      if (node.type === "LogicalExpression") {
        const leftNode = walkAst(node.left);
        const rightNode = walkAst(node.right);
        const currentOp = node.operator === "&&" ? "AND" : "OR";

        const children: Array<VisualLogicGroup | VisualCondition> = [];

        // 合并左侧同类逻辑，保持扁平
        if (leftNode.type === currentOp) {
          children.push(...leftNode.children);
        } else {
          children.push(leftNode);
        }

        // 合并右侧同类逻辑，保持扁平
        if (rightNode.type === currentOp) {
          children.push(...rightNode.children);
        } else {
          children.push(rightNode);
        }

        return {
          id:
            node.operator === "&&"
              ? `and-${Math.random().toString(36).substr(2, 5)}`
              : `or-${Math.random().toString(36).substr(2, 5)}`,
          type: currentOp,
          children,
        };
      }

      if (node.type === "BinaryExpression") {
        // 解析例如：ctx.field === "val"
        let field = "";
        if (node.left.type === "MemberExpression") {
          if (
            node.left.object.type === "Identifier" &&
            node.left.object.name === "ctx"
          ) {
            if (node.left.property.type === "Identifier") {
              field = node.left.property.name;
            }
          }
        }

        let rawOp = node.operator;
        let operator: "==" | "!=" | "starts_with" | "ends_with" = "==";
        if (rawOp === "===" || rawOp === "==") {
          operator = "==";
        } else if (rawOp === "!==" || rawOp === "!=") {
          operator = "!=";
        }

        let value = "";
        if (node.right.type === "Literal") {
          value = String(node.right.value);
        }

        return {
          id: `cond-ts-${Math.random().toString(36).substr(2, 5)}`,
          type: "CONDITION",
          field: (field || "comm") as any,
          operator,
          value,
        };
      }

      if (node.type === "CallExpression") {
        // 解析例如：ctx.field.includes("val") 或 ctx.field.startsWith("val") 或 ctx.field.endsWith("val")
        let field = "";
        let methodName = "";

        // 1. 匹配标准链式调用：ctx.field.startsWith("val")
        if (node.callee.type === "MemberExpression") {
          methodName = node.callee.property.name;
          const obj = node.callee.object;
          if (obj.type === "MemberExpression") {
            if (obj.object.type === "Identifier" && obj.object.name === "ctx") {
              field = obj.property.name;
            }
          } else if (obj.type === "Identifier" && obj.name !== "ctx") {
            // 支持直接在解构后的局部变量上调用，例如 comm.startsWith("val")
            field = obj.name;
          }
        }

        let operator: "starts_with" | "ends_with" | "== " | "==" =
          "starts_with";
        if (methodName === "endsWith" || methodName === "ends_with") {
          operator = "ends_with";
        } else if (methodName === "includes") {
          // 模糊匹配底层转译映射为 starts_with 拦截
          operator = "starts_with";
        } else if (
          methodName === "startsWith" ||
          methodName === "starts_with"
        ) {
          operator = "starts_with";
        }

        let value = "";
        if (
          node.arguments &&
          node.arguments[0] &&
          node.arguments[0].type === "Literal"
        ) {
          value = String(node.arguments[0].value);
        }

        return {
          id: `cond-ts-${Math.random().toString(36).substr(2, 5)}`,
          type: "CONDITION",
          field: (field || "comm") as any,
          operator,
          value,
        };
      }

      // 兜底返回默认空条件
      return {
        id: `cond-ts-default-${Math.random().toString(36).substr(2, 5)}`,
        type: "CONDITION",
        field: "comm",
        operator: "==",
        value: "",
      };
    };

    try {
      // 使用 acorn 解析单个表达式
      const ast = acorn.parseExpressionAt(condStr, 0, {
        ecmaVersion: 2020,
      }) as any;
      const visualNode = walkAst(ast);

      if (visualNode.type === "CONDITION") {
        result.conditions = {
          id: "root",
          type: "AND",
          children: [visualNode],
        };
      } else {
        result.conditions = {
          ...visualNode,
          id: "root",
        } as VisualLogicGroup;
      }
    } catch (e) {
      console.warn(
        "Failed to parse pseudo-code conditions tree via Acorn AST, falling back to simple condition:",
        e
      );
      result.conditions = {
        id: "root",
        type: "AND",
        children: [
          {
            id: `cond-ts-default-${Math.random().toString(36).substr(2, 5)}`,
            type: "CONDITION",
            field: "comm",
            operator: "==",
            value: "",
          },
        ],
      };
    }
  }

  return result;
};
