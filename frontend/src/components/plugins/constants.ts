import {
  ThunderboltOutlined,
  FileTextOutlined,
  FileAddOutlined,
  FolderAddOutlined,
  DeleteOutlined,
  LinkOutlined,
  AlertOutlined,
  SafetyCertificateOutlined,
} from "@ant-design/icons-vue";
import type { TriggerOption } from "./types";

export const triggerOptions: TriggerOption[] = [
  {
    value: "process",
    label: "进程创建与加载 (LSM bprm_check)",
    icon: ThunderboltOutlined,
    color: "#1677ff",
  },
  {
    value: "file_open",
    label: "文件或目录被打开 (LSM file_open)",
    icon: FileTextOutlined,
    color: "#1677ff",
  },
  {
    value: "file_create",
    label: "创建物理新文件 (LSM inode_create)",
    icon: FileAddOutlined,
    color: "#1677ff",
  },
  {
    value: "mkdir",
    label: "创建新目录文件夹 (LSM inode_mkdir)",
    icon: FolderAddOutlined,
    color: "#1677ff",
  },
  {
    value: "rmdir",
    label: "删除已有文件夹 (LSM inode_rmdir)",
    icon: DeleteOutlined,
    color: "#1677ff",
  },
  {
    value: "symlink",
    label: "创建软链接指引 (LSM inode_symlink)",
    icon: LinkOutlined,
    color: "#1677ff",
  },
  {
    value: "unlink",
    label: "删除物理文件对象 (Kprobe unlink)",
    icon: AlertOutlined,
    color: "#1677ff",
  },
  {
    value: "socket_connect",
    label: "外发 socket 连接拦截 (LSM socket_connect)",
    icon: LinkOutlined,
    color: "#1677ff",
  },
  {
    value: "inode_mknod",
    label: "物理特权设备节点创建 (LSM inode_mknod)",
    icon: FileAddOutlined,
    color: "#1677ff",
  },
  {
    value: "file_mprotect",
    label: "高危内存执行权限修改 (LSM file_mprotect)",
    icon: SafetyCertificateOutlined,
    color: "#1677ff",
  },
  {
    value: "inode_rename",
    label: "关键文件路径重命名 (LSM inode_rename)",
    icon: FileTextOutlined,
    color: "#1677ff",
  },
];

export const fieldOptions = [
  { value: "comm", label: "当前进程名称 (Comm)" },
  { value: "pid", label: "当前进程 PID" },
  { value: "uid", label: "当前进程用户 UID" },
  { value: "basename", label: "操作目标文件名 (Basename)" },
  { value: "port", label: "目标网络端口 (Port)" },
  { value: "ipv4", label: "目标 IPv4 地址 (IPv4)" },
  { value: "gid", label: "当前进程组 GID" },
];
