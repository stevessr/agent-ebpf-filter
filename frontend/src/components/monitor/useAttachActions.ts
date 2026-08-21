/**
 * Attach-action helpers for the process observer: resolves binary paths and
 * drives per-PID TLS attach buttons (auto-detected builtins / Go / library).
 */
import { reactive } from "vue";
import axios from "axios";
import type { ComputedRef } from "vue";
import type { ProcessInfo } from "../../composables/monitor/useProcessObserver";

// Axios error shape we read for user-facing messages (library boundary).
type AttachErrorLike = {
  response?: { data?: { error?: string } };
  message?: string;
};

const attachErrorMessage = (e: unknown, fallback: string): string => {
  // Axios throws AxiosError instances; read response.data.error off them.
  const like = e as AttachErrorLike;
  return like?.response?.data?.error || like?.message || fallback;
};

export const useAttachActions = (deps: {
  treeProcessByPid: ComputedRef<Map<number, ProcessInfo>>;
  treeSSLPending: ComputedRef<ProcessInfo[]>;
  fetchAttachedPIDs: () => Promise<void> | void;
}) => {
  const attachingPids = reactive(new Set<number>());
  const attachErrors = reactive<Record<number, string>>({});

  const getBinaryPath = async (pid: number): Promise<string> => {
    // Always resolve via /proc/PID/exe for the real binary, not cmdline
    try {
      const res = await axios.get("/system/process/exe", { params: { pid } });
      return res.data.path || "";
    } catch {
      // Fallback: try cmdline
      const p = deps.treeProcessByPid.value.get(pid);
      if (p?.cmdline) {
        const parts = p.cmdline.split(/\s+/);
        if (parts[0]) return parts[0];
      }
      return "";
    }
  };

  const doAttachBuiltins = async (pid: number) => {
    if (attachingPids.has(pid)) return;
    attachingPids.add(pid);
    delete attachErrors[pid];
    try {
      const exePath = await getBinaryPath(pid);
      if (!exePath) {
        attachErrors[pid] = "Cannot resolve binary path for PID " + pid;
        return;
      }
      // Use executable API which auto-detects: Go uprobes → static SSL → library (openssl/gnutls)
      const res = await axios.post("/tls-capture/executable", {
        path: exePath,
        pid,
        library: "",
      });
      if (res.data.result?.error) {
        attachErrors[pid] = res.data.result.error;
      } else {
        await deps.fetchAttachedPIDs();
      }
    } catch (e) {
      attachErrors[pid] = attachErrorMessage(e, "Auto-attach failed");
    } finally {
      attachingPids.delete(pid);
    }
  };

  const doAttachGo = async (pid: number) => {
    if (attachingPids.has(pid)) return;
    attachingPids.add(pid);
    delete attachErrors[pid];
    try {
      const path = await getBinaryPath(pid);
      if (!path) {
        attachErrors[pid] = "Cannot determine binary path for PID " + pid;
        return;
      }
      await axios.post("/tls-capture/go-binary", { path, pid });
      await deps.fetchAttachedPIDs();
    } catch (e) {
      attachErrors[pid] = attachErrorMessage(e, "Go attach failed");
    } finally {
      attachingPids.delete(pid);
    }
  };

  const doAttachLibrary = async (pid: number, library: string) => {
    if (attachingPids.has(pid)) return;
    attachingPids.add(pid);
    delete attachErrors[pid];
    try {
      const path = await getBinaryPath(pid);
      if (!path) {
        attachErrors[pid] = "Cannot determine binary path for PID " + pid;
        return;
      }
      await axios.post("/tls-capture/executable", { path, pid, library });
      await deps.fetchAttachedPIDs();
    } catch (e) {
      attachErrors[pid] = attachErrorMessage(e, "Library attach failed");
    } finally {
      attachingPids.delete(pid);
    }
  };

  const doAttachAllBuiltins = async () => {
    const pids = deps.treeSSLPending.value.map((p) => p.pid);
    await Promise.all(pids.map((pid) => doAttachBuiltins(pid)));
  };

  return {
    attachingPids,
    attachErrors,
    getBinaryPath,
    doAttachBuiltins,
    doAttachGo,
    doAttachLibrary,
    doAttachAllBuiltins,
  };
};
