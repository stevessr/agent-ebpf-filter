import { computed, ref } from "vue";
import { message } from "ant-design-vue";
import type {
  ShellSessionCreateRequest,
  ShellSessionInfo,
} from "../../types/shell";
import {
  splitArgs,
  basename,
  dirname,
  resolvePythonInterpreter,
  splitRuntimeAndScriptArgs,
} from "./useLauncherUtils";

export type ScriptLanguage =
  | "python"
  | "node"
  | "ruby"
  | "sh"
  | "pwsh"
  | "deno"
  | "bun";

export interface ScriptLaunchPlan {
  command: string;
  args: string[];
  preview: string;
}

export const SCRIPT_LANGUAGE_OPTIONS: Array<{
  label: string;
  value: ScriptLanguage;
}> = [
  { label: "Python", value: "python" },
  { label: "Node.js", value: "node" },
  { label: "Ruby", value: "ruby" },
  { label: "Shell (sh)", value: "sh" },
  { label: "PowerShell (pwsh)", value: "pwsh" },
  { label: "Deno", value: "deno" },
  { label: "Bun", value: "bun" },
];

export const resolveScriptLaunchPlan = (
  language: ScriptLanguage,
  venvPath: string,
  scriptPath: string,
  rawArgs: string,
): ScriptLaunchPlan => {
  const script = scriptPath.trim();
  const scriptDisplay = script || "<script>";
  const tokens = splitArgs(rawArgs);

  switch (language) {
    case "python":
      return {
        command: resolvePythonInterpreter(venvPath),
        args: [scriptDisplay, ...tokens],
        preview: [
          resolvePythonInterpreter(venvPath),
          scriptDisplay,
          ...tokens,
        ].join(" "),
      };
    case "node":
      return {
        command: "node",
        args: [scriptDisplay, ...tokens],
        preview: ["node", scriptDisplay, ...tokens].join(" "),
      };
    case "ruby":
      return {
        command: "ruby",
        args: [scriptDisplay, ...tokens],
        preview: ["ruby", scriptDisplay, ...tokens].join(" "),
      };
    case "sh":
      return {
        command: "sh",
        args: [scriptDisplay, ...tokens],
        preview: ["sh", scriptDisplay, ...tokens].join(" "),
      };
    case "pwsh": {
      const { runtimeArgs, scriptArgs } = splitRuntimeAndScriptArgs(rawArgs);
      return {
        command: "pwsh",
        args: [...runtimeArgs, "-File", scriptDisplay, ...scriptArgs],
        preview: [
          "pwsh",
          ...runtimeArgs,
          "-File",
          scriptDisplay,
          ...scriptArgs,
        ].join(" "),
      };
    }
    case "deno": {
      const { runtimeArgs, scriptArgs } = splitRuntimeAndScriptArgs(rawArgs);
      return {
        command: "deno",
        args: ["run", ...runtimeArgs, scriptDisplay, ...scriptArgs],
        preview: [
          "deno",
          "run",
          ...runtimeArgs,
          scriptDisplay,
          ...scriptArgs,
        ].join(" "),
      };
    }
    case "bun":
      return {
        command: "bun",
        args: [scriptDisplay, ...tokens],
        preview: ["bun", scriptDisplay, ...tokens].join(" "),
      };
    default:
      return {
        command: resolvePythonInterpreter(venvPath),
        args: [scriptDisplay, ...tokens],
        preview: [
          resolvePythonInterpreter(venvPath),
          scriptDisplay,
          ...tokens,
        ].join(" "),
      };
  }
};

export function useScriptLauncher(
  createSession: (
    payload: ShellSessionCreateRequest,
    successMessage: string,
  ) => Promise<ShellSessionInfo | undefined>,
  getLaunchEnvRecord: () => Record<string, string>,
) {
  const scriptLanguage = ref<ScriptLanguage>("python");
  const scriptPath = ref("");
  const scriptWorkDir = ref("");
  const pythonVenv = ref("");
  const scriptArgs = ref("");
  const scriptLaunching = ref(false);

  const scriptArgsPlaceholder = computed(() => {
    switch (scriptLanguage.value) {
      case "deno":
        return "--allow-read -- --foo bar";
      case "pwsh":
        return "-ExecutionPolicy Bypass -- --foo bar";
      case "bun":
        return "--foo bar";
      default:
        return "--debug --foo bar";
    }
  });

  const scriptCommandPreview = computed(() => {
    return resolveScriptLaunchPlan(
      scriptLanguage.value,
      pythonVenv.value,
      scriptPath.value,
      scriptArgs.value,
    ).preview;
  });

  const launchScript = async () => {
    const script = scriptPath.value.trim();
    if (!script) {
      message.error("Please choose a script file");
      return;
    }

    const workDir = scriptWorkDir.value.trim() || dirname(script);
    const launchPlan = resolveScriptLaunchPlan(
      scriptLanguage.value,
      pythonVenv.value,
      script,
      scriptArgs.value,
    );

    scriptLaunching.value = true;
    try {
      const payload: ShellSessionCreateRequest = {
        shell: scriptLanguage.value,
        command: launchPlan.command,
        args: launchPlan.args,
        workDir,
        cols: 100,
        rows: 32,
        label: `${scriptLanguage.value}: ${basename(script)}`,
        kind: "script",
        env: getLaunchEnvRecord(),
      };
      await createSession(
        payload,
        `Launched ${scriptLanguage.value} script: ${basename(script)}`,
      );
    } catch (err: any) {
      message.error(
        err?.response?.data?.error || err?.message || "Failed to launch script",
      );
    } finally {
      scriptLaunching.value = false;
    }
  };

  return {
    scriptLanguage,
    scriptPath,
    scriptWorkDir,
    pythonVenv,
    scriptArgs,
    scriptLaunching,
    scriptArgsPlaceholder,
    scriptCommandPreview,
    launchScript,
  };
}
