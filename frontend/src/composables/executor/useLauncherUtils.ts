export const splitArgs = (input: string) => {
  const output: string[] = [];
  let current = "";
  let quote: '"' | "'" | null = null;
  let escaped = false;

  for (const char of input.trim()) {
    if (escaped) {
      current += char;
      escaped = false;
      continue;
    }

    if (char === "\\") {
      escaped = true;
      continue;
    }

    if (quote) {
      if (char === quote) {
        quote = null;
        continue;
      }
      current += char;
      continue;
    }

    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }

    if (/\s/.test(char)) {
      if (current) {
        output.push(current);
        current = "";
      }
      continue;
    }

    current += char;
  }

  if (escaped) {
    current += "\\";
  }

  if (current) {
    output.push(current);
  }

  return output;
};

export const basename = (path: string) => {
  const normalized = path.trim().replace(/\/+$/, "");
  if (!normalized || normalized === "/") return "/";
  const index = normalized.lastIndexOf("/");
  return index >= 0 ? normalized.slice(index + 1) || normalized : normalized;
};

export const dirname = (path: string) => {
  const normalized = path.trim().replace(/\/+$/, "");
  if (!normalized || normalized === "/") return "/";
  const index = normalized.lastIndexOf("/");
  if (index <= 0) return "/";
  return normalized.slice(0, index);
};

export const sanitizeTmuxSessionName = (value: string) => {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^[-.]+|[-.]+$/g, "");
  return slug || "coding-cli";
};

export const resolvePythonInterpreter = (venvPath: string) => {
  const normalized = venvPath.trim().replace(/\/+$/, "");
  if (!normalized) return "python3";
  if (
    normalized.endsWith("/python") ||
    normalized.endsWith("/python3") ||
    normalized.endsWith("/python.exe")
  ) {
    return normalized;
  }
  return `${normalized}/bin/python`;
};

export const splitRuntimeAndScriptArgs = (input: string) => {
  const tokens = splitArgs(input);
  const separatorIndex = tokens.indexOf("--");
  if (separatorIndex < 0) {
    return {
      runtimeArgs: [] as string[],
      scriptArgs: tokens,
    };
  }
  return {
    runtimeArgs: tokens.slice(0, separatorIndex),
    scriptArgs: tokens.slice(separatorIndex + 1),
  };
};
