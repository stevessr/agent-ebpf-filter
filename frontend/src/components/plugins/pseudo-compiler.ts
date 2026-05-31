import * as acorn from "acorn";
import type {
  VisualAction,
  VisualCondition,
  VisualConditionField,
  VisualLogicGroup,
  VisualMapKey,
  VisualTrigger,
  VisualWorkspaceSnapshot,
} from "./types";

const triggers: VisualTrigger[] = [
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

const mapKeys: VisualMapKey[] = ["uid", "pid", "comm"];
const conditionFields: VisualConditionField[] = [
  "comm",
  "pid",
  "uid",
  "basename",
  "port",
  "ipv4",
  "gid",
  "ppid",
  "loginuid",
];

const isTrigger = (value: string): value is VisualTrigger =>
  triggers.includes(value as VisualTrigger);

const isMapKey = (value: string): value is VisualMapKey =>
  mapKeys.includes(value as VisualMapKey);

const isConditionField = (value: string): value is VisualConditionField =>
  conditionFields.includes(value as VisualConditionField);

const literalToString = (node: any): string => {
  if (!node) return "";
  if (node.type === "Literal") return String(node.value ?? "");
  if (node.type === "TemplateLiteral" && node.quasis?.length === 1) {
    return String(node.quasis[0]?.value?.cooked ?? "");
  }
  return "";
};

const normalizeField = (field: string): VisualConditionField =>
  isConditionField(field) ? field : "comm";

export const createPseudoSeedSnapshot = (
  pluginId = "ts-pseudocode-filter",
  pluginName = "TS 伪代码插件",
  description = "由独立 TS 伪代码工作区生成的 eBPF 过滤审计插件。",
): VisualWorkspaceSnapshot => ({
  version: 1,
  trigger: "process",
  action: "BLOCK",
  conditions: {
    id: "root",
    type: "AND",
    children: [
      {
        id: "cond-pseudo-default",
        type: "CONDITION",
        field: "comm",
        operator: "==",
        value: "nc",
      },
    ],
  },
  mapMode: "NONE",
  mapKey: "pid",
  mapLimit: 10,
  pluginId,
  pluginName,
  description,
});

/**
 * Parse TS-style pseudocode into the internal BPF generator input.
 * This does not mutate or hydrate the visual canvas workspace.
 */
export const pseudoCodeToBpfSnapshot = (
  code: string,
  seedSnapshot: VisualWorkspaceSnapshot,
): VisualWorkspaceSnapshot => {
  const result: VisualWorkspaceSnapshot = JSON.parse(
    JSON.stringify(seedSnapshot),
  ) as VisualWorkspaceSnapshot;
  let idCounter = 0;
  const nextId = (prefix: string) => `${prefix}-${++idCounter}`;

  const importMatch = code.match(/import\s*\{\s*(\w+)/);
  if (importMatch?.[1] && isTrigger(importMatch[1].trim())) {
    result.trigger = importMatch[1].trim() as VisualTrigger;
  }

  const actionMatch = code.match(/Action\.(\w+)\s*\(\s*\)/i);
  if (actionMatch?.[1]) {
    const action = actionMatch[1].toUpperCase() as VisualAction;
    if (["BLOCK", "ALERT", "KILL"].includes(action)) {
      result.action = action;
    }
  }

  if (code.includes("Maps.createCounter")) {
    result.mapMode = "COUNTER";
    const keyMatch = code.match(
      /Maps\.createCounter\(\{\s*key:\s*["'](\w+)["']/,
    );
    if (keyMatch?.[1] && isMapKey(keyMatch[1])) result.mapKey = keyMatch[1];
    const limitMatch = code.match(/limit:\s*(\d+)/);
    if (limitMatch?.[1]) result.mapLimit = parseInt(limitMatch[1], 10);
  } else if (code.includes("Maps.createBlocklist")) {
    result.mapMode = "BLOCKLIST";
    const keyMatch = code.match(
      /Maps\.createBlocklist\(\{\s*key:\s*["'](\w+)["']/,
    );
    if (keyMatch?.[1] && isMapKey(keyMatch[1])) result.mapKey = keyMatch[1];
  } else {
    result.mapMode = "NONE";
  }

  const ifMatch = code.match(/if\s*\(([\s\S]*?)\)\s*\{/);
  if (!ifMatch?.[1]) return result;

  const walkAst = (node: any): VisualLogicGroup | VisualCondition => {
    if (node.type === "LogicalExpression") {
      const currentOp = node.operator === "&&" ? "AND" : "OR";
      const leftNode = walkAst(node.left);
      const rightNode = walkAst(node.right);
      const children: Array<VisualLogicGroup | VisualCondition> = [];

      if (leftNode.type === currentOp) children.push(...leftNode.children);
      else children.push(leftNode);
      if (rightNode.type === currentOp) children.push(...rightNode.children);
      else children.push(rightNode);

      return {
        id: nextId(currentOp.toLowerCase()),
        type: currentOp,
        children,
      };
    }

    if (node.type === "BinaryExpression") {
      let field = "comm";
      if (
        node.left?.type === "MemberExpression" &&
        node.left.object?.type === "Identifier" &&
        node.left.object.name === "ctx"
      ) {
        field = node.left.property?.name || "comm";
      } else if (node.left?.type === "Identifier") {
        field = node.left.name;
      }

      let operator: VisualCondition["operator"] = "==";
      if (node.operator === "!==" || node.operator === "!=") {
        operator = "!=";
      }

      return {
        id: nextId("cond-ts"),
        type: "CONDITION",
        field: normalizeField(field),
        operator,
        value: literalToString(node.right),
      };
    }

    if (node.type === "CallExpression") {
      let field = "comm";
      let methodName = "";
      if (node.callee?.type === "MemberExpression") {
        methodName = node.callee.property?.name || "";
        const obj = node.callee.object;
        if (
          obj?.type === "MemberExpression" &&
          obj.object?.type === "Identifier" &&
          obj.object.name === "ctx"
        ) {
          field = obj.property?.name || "comm";
        } else if (obj?.type === "Identifier" && obj.name !== "ctx") {
          field = obj.name;
        }
      }

      let operator: VisualCondition["operator"] = "starts_with";
      if (methodName === "endsWith" || methodName === "ends_with") {
        operator = "ends_with";
      } else if (
        methodName === "startsWith" ||
        methodName === "starts_with" ||
        methodName === "includes"
      ) {
        operator = "starts_with";
      }

      return {
        id: nextId("cond-ts"),
        type: "CONDITION",
        field: normalizeField(field),
        operator,
        value: literalToString(node.arguments?.[0]),
      };
    }

    return {
      id: nextId("cond-ts-default"),
      type: "CONDITION",
      field: "comm",
      operator: "==",
      value: "",
    };
  };

  try {
    const ast = acorn.parseExpressionAt(ifMatch[1].trim(), 0, {
      ecmaVersion: 2020,
    }) as any;
    const visualNode = walkAst(ast);
    result.conditions =
      visualNode.type === "CONDITION"
        ? { id: "root", type: "AND", children: [visualNode] }
        : ({ ...visualNode, id: "root" } as VisualLogicGroup);
  } catch (error) {
    console.warn("Failed to parse TS pseudocode condition expression", error);
  }

  return result;
};
