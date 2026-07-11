import { ref, type Ref } from "vue";
import axios from "axios";

export function useProgramLauncher(options: {
  addPid: (pid: number) => void;
  pidInput: Ref<string>;
}) {
  const { addPid, pidInput } = options;
  const launchModalOpen = ref(false);

  const launchPath = ref("");
  const launchUser = ref("");
  const launchCwd = ref("");
  const launchArgs = ref("");
  const launching = ref(false);
  const launchError = ref("");

  // File/dir browser state
  type BrowseTarget = "program" | "cwd";
  const browserTarget = ref<BrowseTarget>("program");
  const browserOpen = ref(false);
  const browserStartPath = ref("/");

  // User list state
  interface SysUser {
    username: string;
    uid: number;
    home: string;
    shell: string;
  }
  const sysUsers = ref<SysUser[]>([]);
  const usersLoading = ref(false);

  // Recent launches (localStorage persistence)
  interface RecentLaunch {
    program: string;
    user: string;
    cwd: string;
    args: string;
  }
  const STORAGE_KEY = "observe-recent-launches";
  const MAX_RECENT = 10;

  const loadRecentLaunches = (): RecentLaunch[] => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      return raw ? JSON.parse(raw) : [];
    } catch {
      return [];
    }
  };
  const recentLaunches = ref<RecentLaunch[]>(loadRecentLaunches());

  const saveRecentLaunch = (rl: RecentLaunch) => {
    const existing = loadRecentLaunches().filter(
      (r) => r.program !== rl.program || r.args !== rl.args,
    );
    existing.unshift(rl);
    if (existing.length > MAX_RECENT) existing.length = MAX_RECENT;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(existing));
    recentLaunches.value = existing;
  };

  const applyRecent = (rl: RecentLaunch) => {
    launchPath.value = rl.program;
    launchUser.value = rl.user;
    launchCwd.value = rl.cwd;
    launchArgs.value = rl.args;
  };

  const fetchUserInfo = async () => {
    try {
      const res = await axios.get("/system/user-info");
      launchCwd.value = res.data.home || "/tmp";
      launchUser.value = res.data.username || "";
    } catch {
      launchCwd.value = "/tmp";
      launchUser.value = "";
    }
  };

  const fetchUsers = async () => {
    usersLoading.value = true;
    try {
      const res = await axios.get("/system/users");
      sysUsers.value = Array.isArray(res.data) ? res.data : [];
    } catch {
      sysUsers.value = [];
    } finally {
      usersLoading.value = false;
    }
  };

  fetchUserInfo();
  fetchUsers();

  const openBrowser = (target: BrowseTarget) => {
    browserTarget.value = target;
    browserStartPath.value =
      target === "program"
        ? launchPath.value
          ? launchPath.value.split("/").slice(0, -1).join("/") || "/"
          : "/usr/bin"
        : launchCwd.value || "/";
    browserOpen.value = true;
  };

  const onBrowserSelect = (path: string) => {
    if (browserTarget.value === "program") {
      launchPath.value = path;
      // Auto-fill cwd from program directory if cwd is empty
      if (!launchCwd.value.trim()) {
        launchCwd.value = path.split("/").slice(0, -1).join("/") || "/";
      }
    } else {
      launchCwd.value = path;
    }
  };

  const doLaunch = async () => {
    if (!launchPath.value.trim()) return;
    launching.value = true;
    launchError.value = "";
    try {
      const args = launchArgs.value.split(/\s+/).filter((a) => a.length > 0);
      const res = await axios.post("/system/run", {
        comm: launchPath.value.trim(),
        args,
        user: launchUser.value.trim() || undefined,
        cwd: launchCwd.value.trim() || undefined,
      });
      if (res.data.pid) {
        addPid(res.data.pid);
        pidInput.value = "";
        // Persist to localStorage
        saveRecentLaunch({
          program: launchPath.value.trim(),
          user: launchUser.value.trim(),
          cwd: launchCwd.value.trim(),
          args: launchArgs.value.trim(),
        });
      }
    } catch (e: any) {
      launchError.value =
        e?.response?.data?.error || e?.message || "Launch failed";
    } finally {
      launching.value = false;
    }
  };

  return {
    launchModalOpen,
    launchPath,
    launchUser,
    launchCwd,
    launchArgs,
    launching,
    launchError,
    browserTarget,
    browserOpen,
    browserStartPath,
    sysUsers,
    usersLoading,
    recentLaunches,
    applyRecent,
    openBrowser,
    onBrowserSelect,
    doLaunch,
  };
}
