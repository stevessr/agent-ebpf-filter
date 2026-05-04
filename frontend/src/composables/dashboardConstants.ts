import { pb } from "../pb/tracker_pb.js";

export interface AgentEvent {
  key: string;
  pid: number;
  ppid: number;
  uid: number;
  type: string;
  eventType?: number;
  tag: string;
  comm: string;
  path: string;
  netDirection?: string;
  netEndpoint?: string;
  netFamily?: string;
  netBytes?: number;
  retval?: number;
  extraInfo?: string;
  extraPath?: string;
  bytes?: number;
  mode?: string;
  domain?: string;
  sockType?: string;
  protocol?: number;
  uidArg?: number;
  gidArg?: number;
  time: string;
  receivedAtMs?: number;
  occurrenceCount?: number;
}

export interface BuiltinFilterRule {
  id: string;
  label: string;
  test: (event: AgentEvent) => boolean;
}

type ResizableColumnKey = 'time' | 'tag' | 'pid' | 'comm' | 'type' | 'path' | 'action';

export const minColumnWidths: Record<ResizableColumnKey, number> = {
  time: 100,
  tag: 100,
  pid: 88,
  comm: 120,
  type: 120,
  path: 160,
  action: 72,
};

export const eventTypes = [
  'execve',
  'openat',
  'network_connect',
  'network_bind',
  'network_sendto',
  'network_recvfrom',
  'mkdir',
  'unlink',
  'ioctl',
  'read',
  'write',
  'open',
  'chmod',
  'chown',
  'rename',
  'link',
  'symlink',
  'mknod',
  'clone',
  'exit',
  'socket',
  'accept',
  'accept4',
  'syscall',
  'wrapper_intercept',
  'native_hook',
];

export const pageSizeOptions = ['20', '50', '100', '200'];

export const eventTypeLabelMap: Record<number, string> = {
  [pb.EventType.EXECVE]: 'execve',
  [pb.EventType.OPENAT]: 'openat',
  [pb.EventType.NETWORK_CONNECT]: 'network_connect',
  [pb.EventType.MKDIR]: 'mkdir',
  [pb.EventType.UNLINK]: 'unlink',
  [pb.EventType.IOCTL]: 'ioctl',
  [pb.EventType.NETWORK_BIND]: 'network_bind',
  [pb.EventType.NETWORK_SENDTO]: 'network_sendto',
  [pb.EventType.NETWORK_RECVFROM]: 'network_recvfrom',
  [pb.EventType.READ]: 'read',
  [pb.EventType.WRITE]: 'write',
  [pb.EventType.OPEN]: 'open',
  [pb.EventType.CHMOD]: 'chmod',
  [pb.EventType.CHOWN]: 'chown',
  [pb.EventType.RENAME]: 'rename',
  [pb.EventType.LINK]: 'link',
  [pb.EventType.SYMLINK]: 'symlink',
  [pb.EventType.MKNOD]: 'mknod',
  [pb.EventType.CLONE]: 'clone',
  [pb.EventType.EXIT]: 'exit',
  [pb.EventType.SOCKET]: 'socket',
  [pb.EventType.ACCEPT]: 'accept',
  [pb.EventType.ACCEPT4]: 'accept4',
  25: 'syscall',
  [pb.EventType.WRAPPER_INTERCEPT]: 'wrapper_intercept',
  [pb.EventType.NATIVE_HOOK]: 'native_hook',
};

export const eventTypeColorMap: Record<number, string> = {
  [pb.EventType.EXECVE]: 'blue',
  [pb.EventType.OPENAT]: 'green',
  [pb.EventType.NETWORK_CONNECT]: 'orange',
  [pb.EventType.MKDIR]: 'cyan',
  [pb.EventType.UNLINK]: 'red',
  [pb.EventType.IOCTL]: 'purple',
  [pb.EventType.NETWORK_BIND]: 'volcano',
  [pb.EventType.NETWORK_SENDTO]: 'cyan',
  [pb.EventType.NETWORK_RECVFROM]: 'geekblue',
  [pb.EventType.READ]: 'cyan',
  [pb.EventType.WRITE]: 'cyan',
  [pb.EventType.OPEN]: 'green',
  [pb.EventType.CHMOD]: 'gold',
  [pb.EventType.CHOWN]: 'gold',
  [pb.EventType.RENAME]: 'orange',
  [pb.EventType.LINK]: 'orange',
  [pb.EventType.SYMLINK]: 'orange',
  [pb.EventType.MKNOD]: 'purple',
  [pb.EventType.CLONE]: 'blue',
  [pb.EventType.EXIT]: 'red',
  [pb.EventType.SOCKET]: 'orange',
  [pb.EventType.ACCEPT]: 'volcano',
  [pb.EventType.ACCEPT4]: 'volcano',
  25: 'geekblue',
};

export const networkEventTypes = new Set<number>([
  pb.EventType.NETWORK_CONNECT,
  pb.EventType.NETWORK_BIND,
  pb.EventType.NETWORK_SENDTO,
  pb.EventType.NETWORK_RECVFROM,
  pb.EventType.ACCEPT,
  pb.EventType.ACCEPT4,
  pb.EventType.SOCKET,
]);

export const eventCategories: Record<string, Set<number>> = {
  network: new Set([
    pb.EventType.NETWORK_CONNECT,
    pb.EventType.NETWORK_BIND,
    pb.EventType.NETWORK_SENDTO,
    pb.EventType.NETWORK_RECVFROM,
    pb.EventType.SOCKET,
    pb.EventType.ACCEPT,
    pb.EventType.ACCEPT4,
  ]),
  file: new Set([
    pb.EventType.OPENAT,
    pb.EventType.IOCTL,
    pb.EventType.READ,
    pb.EventType.WRITE,
    pb.EventType.OPEN,
    pb.EventType.CHMOD,
    pb.EventType.CHOWN,
    pb.EventType.RENAME,
    pb.EventType.LINK,
    pb.EventType.SYMLINK,
    pb.EventType.MKNOD,
    pb.EventType.MKDIR,
    pb.EventType.UNLINK,
  ]),
  process: new Set([
    pb.EventType.EXECVE,
    pb.EventType.CLONE,
    pb.EventType.EXIT,
  ]),
  hook: new Set([
    pb.EventType.WRAPPER_INTERCEPT,
    pb.EventType.NATIVE_HOOK,
  ]),
};

export const categoryTabs = [
  { key: 'all', label: '全部' },
  { key: 'network', label: '网络' },
  { key: 'file', label: '文件' },
  { key: 'process', label: '进程' },
  { key: 'hook', label: '钩子' },
  { key: 'syscall', label: '系统调用' },
] as const;

export const syscallCatLabels: Record<string, string> = {
  io: 'I/O & FS',
  net: 'Network',
  proc: 'Process',
  sig: 'Signal',
  ipc: 'SysV IPC',
  sec: 'Security',
  misc: 'Misc',
  other: 'Other',
};

export const syscallCatColors: Record<string, string> = {
  io: 'cyan', net: 'purple', proc: 'orange', sig: 'red',
  ipc: 'gold', sec: 'red', misc: 'default', other: 'default',
};

export const builtinFilterRules: BuiltinFilterRule[] = [
  {
    id: 'tty',
    label: 'TTY / PTY',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`.toLowerCase();
      const comm = event.comm.toLowerCase();
      return comm === 'tty'
        || path.includes('/dev/tty')
        || path.includes('/dev/pts/');
    },
  },
  {
    id: 'git',
    label: '.git metadata',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`;
      return /(^|\/)\.git(\/|$)/.test(path);
    },
  },
  {
    id: 'temp',
    label: 'Temp / cache',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`;
      return /(^|\/)(?:\.cache|__pycache__)(\/|$)/.test(path)
        || /(?:\.swp|\.tmp|~)$/i.test(path);
    },
  },
  {
    id: 'builddirs',
    label: '.venv / node_modules / target',
    test: (event) => {
      const path = `${event.path ?? ''}\n${event.extraPath ?? ''}`;
      return /(^|\/)(?:\.venv|node_modules|target)(\/|$)/.test(path);
    },
  },
];

export const baseColumns = [
  { title: 'Time', dataIndex: 'time', key: 'time' },
  { title: 'Tag', dataIndex: 'tag', key: 'tag' },
  { title: 'PID', dataIndex: 'pid', key: 'pid' },
  { title: 'Command', dataIndex: 'comm', key: 'comm' },
  { title: 'Event Type', dataIndex: 'type', key: 'type' },
  { title: 'Path', dataIndex: 'path', key: 'path', ellipsis: true },
  { title: 'Action', key: 'action', fixed: 'right' as const },
] as const;

export const parseSyscallNr = (info?: string): number => {
  const m = info?.match(/\((\d+)\)/);
  return m ? Number(m[1]) : 0;
};

export const syscallCategory = (nr: number): string => {
  if (nr >= 0 && nr <= 40) return 'io';       // read,write,open,close,stat,poll,mmap,brk,dup,pipe,nanosleep...
  if (nr >= 41 && nr <= 55) return 'net';      // socket,connect,accept,sendto,recvfrom,bind,listen...
  if (nr >= 56 && nr <= 61) return 'proc';     // clone,fork,vfork,execve,exit
  if (nr >= 62 && nr <= 63) return 'sig';      // kill,uname
  if (nr >= 64 && nr <= 71) return 'ipc';      // System V IPC
  if (nr >= 72 && nr <= 100) return 'io';      // fcntl,flock,fsync,truncate,chdir,mkdir,chmod,chown...
  if (nr >= 101 && nr <= 200) return 'sec';    // ptrace,prctl,capget,setuid,mount,pivot_root,chroot...
  if (nr >= 201 && nr <= 256) return 'misc';   // futex,epoll,timer,clock,mbind,inotify,migrate...
  if (nr >= 257 && nr <= 334) return 'io';     // openat,mkdirat,unlinkat,renameat,execveat,memfd_create...
  if (nr >= 424 && nr <= 453) return 'sec';    // pidfd,io_uring,open_tree,mount,landlock,clone3...
  return 'other';
};

export const syscallDisplayName = (info?: string): string => {
  if (!info) return '';
  const m = info.match(/^(\w+)\(\d+\)/);
  return m ? m[1] : '';
};


export const selectableEventTypes = eventTypes
  .map((label) => {
    const entry = Object.entries(eventTypeLabelMap).find(([, mappedLabel]) => mappedLabel === label);
    return entry ? Number(entry[0]) : undefined;
  })
  .filter((value): value is number => value !== undefined);
