export type FilePreviewType = 'text' | 'image' | 'binary' | 'directory';

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
  content: string;
  dataUrl: string;
  truncated: boolean;
}

const previewableEventTypes = new Set(['execve', 'openat', 'mkdir', 'unlink']);

export const canPreviewEventPath = (event: { type: string; path: string }) =>
  previewableEventTypes.has(event.type) && event.path.startsWith('/');
