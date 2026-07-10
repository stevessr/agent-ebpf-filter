import type * as Monaco from "monaco-editor";

export type MonacoApi = Pick<typeof Monaco, "editor" | "languages">;
export type MonacoTypeScriptApi = typeof Monaco.typescript;

let configured = false;

/**
 * 配置 Monaco Editor 的 TypeScript 编译器选项、虚拟 typings 以及自定义代码补全
 */
export const configureMonacoTypesAndCompletion = (
  monaco: MonacoApi,
  typescript: MonacoTypeScriptApi,
) => {
  if (configured) return;

  try {
    typescript.typescriptDefaults.setCompilerOptions({
      target: typescript.ScriptTarget.ESNext,
      allowNonTsExtensions: true,
      moduleResolution: typescript.ModuleResolutionKind.NodeJs,
      module: typescript.ModuleKind.CommonJS,
    });

    typescript.typescriptDefaults.addExtraLib(
      `
      declare module "ebpf" {
        export interface HookContext {
          comm: string & {
            startsWith(val: string): boolean;
            endsWith(val: string): boolean;
            includes(val: string): boolean;
          };
          pid: number;
          uid: number;
          basename: string & {
            startsWith(val: string): boolean;
            endsWith(val: string): boolean;
            includes(val: string): boolean;
          };
          port: number;
          ipv4: string & {
            startsWith(val: string): boolean;
            endsWith(val: string): boolean;
            includes(val: string): boolean;
          };
          gid: number;
          ppid: number;
          loginuid: number;
        }

        export const process: any;
        export const file_open: any;
        export const mkdir: any;
        export const file_create: any;
        export const rmdir: any;
        export const symlink: any;
        export const unlink: any;
        export const socket_connect: any;
        export const inode_mknod: any;
        export const file_mprotect: any;
        export const inode_rename: any;

        export namespace Action {
          export function block(): void;
          export function alert(): void;
          export function kill(): void;
        }

        export namespace Maps {
          export interface MapConfig {
            key: "uid" | "pid" | "comm";
            limit?: number;
          }
          export interface CounterInstance {
            exceeded(): boolean;
          }
          export interface BlocklistInstance {
            matched(): boolean;
          }
          export function createCounter(config: MapConfig): CounterInstance;
          export function createBlocklist(config: MapConfig): BlocklistInstance;
        }
      }
    `,
      "ts:ebpf.d.ts",
    );

    // 注册自定义 eBPF 自动补全
    monaco.languages.registerCompletionItemProvider("typescript", {
      provideCompletionItems: (model, position) => {
        const word = model.getWordUntilPosition(position);
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        };

        const suggestions: Monaco.languages.CompletionItem[] = [
          {
            label: "ctx.comm",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: "当前触发事件的进程名称 (string)",
            insertText: "ctx.comm",
            range,
          },
          {
            label: "ctx.gid",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: "当前触发事件的用户组 GID (number)",
            insertText: "ctx.gid",
            range,
          },
          {
            label: "ctx.ppid",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: "当前触发事件的父进程 PPID (number)",
            insertText: "ctx.ppid",
            range,
          },
          {
            label: "ctx.loginuid",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation:
              "当前触发事件的登录用户 UID (number, 即使 sudo 切换也保持最初登录 UID)",
            insertText: "ctx.loginuid",
            range,
          },
          {
            label: "ctx.uid",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: "当前触发事件的用户 UID (number, 0 代表 root)",
            insertText: "ctx.uid",
            range,
          },
          {
            label: "ctx.pid",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: "当前触发事件的进程 PID (number)",
            insertText: "ctx.pid",
            range,
          },
          {
            label: "ctx.basename",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: "操作的目标文件名 (string)",
            insertText: "ctx.basename",
            range,
          },
          {
            label: "ctx.port",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation:
              "外发网络的目标端口 (number, 仅在 socket_connect 有效)",
            insertText: "ctx.port",
            range,
          },
          {
            label: "ctx.ipv4",
            kind: monaco.languages.CompletionItemKind.Field,
            documentation:
              "外发网络的目标 IPv4 地址 (string, 仅在 socket_connect 有效)",
            insertText: "ctx.ipv4",
            range,
          },
          {
            label: "Action.block()",
            kind: monaco.languages.CompletionItemKind.Method,
            documentation: "执行内核级硬拦截阻断行为",
            insertText: "Action.block();",
            range,
          },
          {
            label: "Action.alert()",
            kind: monaco.languages.CompletionItemKind.Method,
            documentation: "仅触发 RingBuffer 告警审计，不阻断执行",
            insertText: "Action.alert();",
            range,
          },
          {
            label: "Action.kill()",
            kind: monaco.languages.CompletionItemKind.Method,
            documentation: "发送 SIGKILL (9) 信号强制终止该进程",
            insertText: "Action.kill();",
            range,
          },
          {
            label: "Maps.createCounter",
            kind: monaco.languages.CompletionItemKind.Method,
            documentation: "声明一个自动频控计数器 Map",
            insertText: 'Maps.createCounter({ key: "pid", limit: 3 })',
            range,
          },
          {
            label: "Maps.createBlocklist",
            kind: monaco.languages.CompletionItemKind.Method,
            documentation: "声明一个运行时查表黑名单 Map",
            insertText: 'Maps.createBlocklist({ key: "comm" })',
            range,
          },
        ];

        return { suggestions };
      },
    });
    configured = true;
  } catch (error) {
    configured = false;
    throw error;
  }
};
