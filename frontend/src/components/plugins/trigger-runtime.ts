import type { PluginAttachKind } from "../../composables/usePlugins";
import type { VisualTrigger } from "./types";

export const VISUAL_PROGRAM_NAME = "visual_custom_plugin";
export const PSEUDO_PROGRAM_NAME = "pseudo_code_plugin";

export const getAttachKindForTrigger = (
  trigger: VisualTrigger
): PluginAttachKind => (trigger === "unlink" ? "kprobe" : "lsm");

export const getAttachTargetForTrigger = (trigger: VisualTrigger): string => {
  switch (trigger) {
    case "process":
      return "lsm/bprm_check_security";
    case "file_open":
      return "lsm/file_open";
    case "mkdir":
      return "lsm/inode_mkdir";
    case "file_create":
      return "lsm/inode_create";
    case "rmdir":
      return "lsm/inode_rmdir";
    case "symlink":
      return "lsm/inode_symlink";
    case "socket_connect":
      return "lsm/socket_connect";
    case "inode_mknod":
      return "lsm/inode_mknod";
    case "file_mprotect":
      return "lsm/file_mprotect";
    case "inode_rename":
      return "lsm/inode_rename";
    case "unlink":
      return "do_unlinkat";
    default:
      return "";
  }
};
