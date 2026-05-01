export const behaviorCategoryLabels: Record<string, string> = {
  'UNKNOWN': 'Unknown',
  'FILE_READ': 'File Read',
  'FILE_WRITE': 'File Write',
  'FILE_DELETE': 'File Delete',
  'FILE_PERMISSION': 'Chmod/Chown',
  'NETWORK': 'Network',
  'PROCESS_EXEC': 'Process Exec',
  'PROCESS_KILL': 'Process Kill',
  'SYSTEM_INFO': 'System Info',
  'PACKAGE_MANAGER': 'Package Manager',
  'DATABASE': 'Database',
  'COMPRESSION': 'Archive',
  'DEVELOPMENT': 'Development',
  'CONTAINER': 'Container',
  'SENSITIVE': 'Sensitive',
};

export const behaviorCategoryColors: Record<string, string> = {
  'UNKNOWN': 'default',
  'FILE_READ': 'cyan',
  'FILE_WRITE': 'orange',
  'FILE_DELETE': 'red',
  'FILE_PERMISSION': 'gold',
  'NETWORK': 'purple',
  'PROCESS_EXEC': 'magenta',
  'PROCESS_KILL': 'volcano',
  'SYSTEM_INFO': 'geekblue',
  'PACKAGE_MANAGER': 'lime',
  'DATABASE': 'green',
  'COMPRESSION': 'blue',
  'DEVELOPMENT': 'default',
  'CONTAINER': 'cyan',
  'SENSITIVE': 'red',
};

export function getBehaviorLabel(category: string): string {
  return behaviorCategoryLabels[category] || category;
}

export function getBehaviorColor(category: string): string {
  return behaviorCategoryColors[category] || 'default';
}
