export type FilePreviewType = 'text' | 'image' | 'video' | 'binary' | 'directory';

export interface FilePreviewResponse {
  path: string;
  name: string;
  parentDir: string;
  isDir: boolean;
  size: number;
  mode: string;
  modTime: string;
  mimeType: string;
  previewType: FilePreviewType;
  language?: string;
  content: string;
  dataUrl: string;
  truncated: boolean;
}

const previewableEventTypes = new Set(['execve', 'openat', 'mkdir', 'unlink', 'open', 'chmod', 'chown', 'rename', 'link', 'symlink', 'mknod']);

export const canPreviewEventPath = (event: { type: string; path: string }) =>
  previewableEventTypes.has(event.type) && event.path.startsWith('/');
