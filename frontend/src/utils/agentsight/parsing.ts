import { readAny, safeJsonParse } from "./shared";
import {
  decodeStdioMessage,
  formatStdioExpandedContent,
  isStdioSource,
} from "./stdio";
import type {
  AgentSightEvent,
  ParsedAgentSightEvent,
  ParsedAgentSightEventType,
} from "./types";

function determineParsedType(
  event: AgentSightEvent,
): ParsedAgentSightEventType {
  const source = event.source.toLowerCase();
  const data = event.data;
  const typeText =
    `${event.eventType} ${readAny(data, ["type", "event"], "")}`.toLowerCase();
  if (source === "system" || typeText.includes("system")) return "system";
  if (isStdioSource(source)) return "stdio";
  if (isPromptEvent(data)) return "prompt";
  if (isResponseEvent(source, data)) return "response";
  if (source === "file" || isFileData(data)) return "file";
  if (source === "process" || isProcessData(data)) return "process";
  if (
    source === "policy" ||
    typeText.includes("alert") ||
    typeText.includes("policy")
  )
    return "policy";
  if (source === "agent") return "agent";
  return "ssl";
}

function isPromptEvent(data: any) {
  return Boolean(
    data.model ||
    data.messages ||
    data.prompt ||
    data.inputs ||
    data.query ||
    (data.method === "POST" &&
      data.message_type === "request" &&
      (String(data.path || data.url || "").includes("/v1/") ||
        String(data.path || data.url || "").includes("/api/"))),
  );
}

function isResponseEvent(source: string, data: any) {
  return Boolean(
    data.choices ||
    data.completion ||
    data.response ||
    data.sse_events ||
    data.delta ||
    data.content_block ||
    (source === "sse_processor" && (data.sse_event || data.sse_data_digest)) ||
    (data.message_type === "response" &&
      (data.model || data.usage || data.status_code)),
  );
}

function isFileData(data: any) {
  return (
    data.fd !== undefined ||
    ["open", "read", "write", "close"].includes(
      String(data.operation || "").toLowerCase(),
    ) ||
    String(data.event || "").includes("FILE_") ||
    data.filepath !== undefined ||
    (data.path !== undefined && data.operation !== undefined)
  );
}

function isProcessData(data: any) {
  return (
    data.exec !== undefined ||
    data.exit !== undefined ||
    ["EXEC", "EXIT", "FORK", "CLONE", "PROCESS"].includes(
      String(data.event || "").toUpperCase(),
    ) ||
    data.ppid !== undefined ||
    data.parent_pid !== undefined
  );
}

function contentString(value: any) {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function parsePromptEvent(event: AgentSightEvent): ParsedAgentSightEvent {
  const data = event.data;
  let model = data.model || data.vendor || "AI Request";
  let displayData = data;
  if (data.body && typeof data.body === "string") {
    const parsedBody = safeJsonParse(data.body);
    if (parsedBody) {
      if (parsedBody.model) model = parsedBody.model;
      displayData = { ...data, body: parsedBody };
    }
  }
  return {
    id: event.id,
    timestamp: event.timestamp,
    type: "prompt",
    title: `${data.method || "POST"} ${model}`,
    content: contentString(displayData),
    metadata: {
      ...data,
      model,
      method: data.method || "POST",
      url: `${data.host || ""}${data.path || data.url || ""}`,
      raw: data,
      original_source: event.source,
    },
  };
}

function parseResponseEvent(event: AgentSightEvent): ParsedAgentSightEvent {
  const data = event.data;
  let model = data.model || data.vendor || "AI Response";
  if (Array.isArray(data.sse_events)) {
    for (const sseEvent of data.sse_events) {
      if (sseEvent.parsed_data?.message?.model) {
        model = sseEvent.parsed_data.message.model;
        break;
      }
    }
  }
  return {
    id: event.id,
    timestamp: event.timestamp,
    type: "response",
    title: model,
    content: contentString(data.body || data),
    metadata: { ...data, model, raw: data, original_source: event.source },
  };
}

function parseGenericEvent(
  event: AgentSightEvent,
  type: ParsedAgentSightEventType,
): ParsedAgentSightEvent {
  return {
    id: event.id,
    timestamp: event.timestamp,
    type,
    title: event.title,
    content: contentString(event.data),
    metadata: {
      ...event.data,
      original_source: event.source,
      event_type: event.eventType,
    },
  };
}

function parseStdioEvent(event: AgentSightEvent): ParsedAgentSightEvent {
  const decoded = decodeStdioMessage(event.data);
  return {
    id: event.id,
    timestamp: event.timestamp,
    type: "stdio",
    title: decoded.title,
    content: formatStdioExpandedContent(decoded),
    metadata: {
      ...event.data,
      original_source: event.source,
      stdio_kind: decoded.kind,
      rpc_method: decoded.method,
      rpc_id: decoded.id,
      tool_name: decoded.toolName,
      summary: decoded.summary,
      parsed_payload: decoded.parsedPayload,
    },
  };
}

export function parseAgentSightEvent(
  event: AgentSightEvent,
): ParsedAgentSightEvent | null {
  const type = determineParsedType(event);
  if (type === "system") return null;
  if (type === "prompt") return parsePromptEvent(event);
  if (type === "response") return parseResponseEvent(event);
  if (type === "stdio") return parseStdioEvent(event);
  return parseGenericEvent(event, type);
}

function extractPromptContent(data: any) {
  if (data?.body && typeof data.body === "string")
    return safeJsonParse(data.body);
  if (data?.messages || data?.prompt) return data;
  return null;
}

function formatPromptForDiff(obj: any) {
  if (!obj) return "";
  if (Array.isArray(obj.messages)) {
    return obj.messages
      .map((msg: any, index: number) => {
        const role = msg.role || "unknown";
        const content = msg.content;
        if (Array.isArray(content)) {
          const text = content
            .filter((item) => item.type === "text")
            .map((item) => item.text)
            .join("\n");
          return `[${index}] ${String(role).toUpperCase()}:\n${text}`;
        }
        return `[${index}] ${String(role).toUpperCase()}: ${typeof content === "string" ? content : JSON.stringify(content)}`;
      })
      .join("\n\n---\n\n");
  }
  return JSON.stringify(obj, null, 2);
}

function generateSimpleDiff(oldText: string, newText: string) {
  if (oldText === newText) return "No changes detected";
  const oldLines = oldText.split("\n");
  const newLines = newText.split("\n");
  const max = Math.max(oldLines.length, newLines.length);
  const out: string[] = [];
  for (let index = 0; index < max; index += 1) {
    const oldLine = oldLines[index];
    const newLine = newLines[index];
    if (oldLine === newLine) {
      if (out.length < 12) out.push(`  ${oldLine || ""}`);
      continue;
    }
    if (oldLine !== undefined) out.push(`- ${oldLine}`);
    if (newLine !== undefined) out.push(`+ ${newLine}`);
    if (out.length > 80) {
      out.push("  ...");
      break;
    }
  }
  return out.join("\n");
}

export function comparePrompts(oldPrompt: any, newPrompt: any) {
  const oldContent = extractPromptContent(oldPrompt);
  const newContent = extractPromptContent(newPrompt);
  if (!oldContent || !newContent)
    return {
      diff: "Unable to extract prompt content",
      summary: "Unable to compare prompts",
      hasChanges: false,
    };
  const oldText = formatPromptForDiff(oldContent);
  const newText = formatPromptForDiff(newContent);
  const hasChanges = oldText !== newText;
  return {
    diff: generateSimpleDiff(oldText, newText),
    summary: hasChanges
      ? "Prompt changed from previous request"
      : "No changes detected",
    hasChanges,
  };
}
