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

// ── ML score helpers ──

export function getMLScoreColor(score: number): string {
  if (score >= 0.85) return '#52c41a';
  if (score >= 0.60) return '#faad14';
  return '#ff4d4f';
}

export function getAnomalyScoreColor(score: number): string {
  if (score <= 0.30) return '#52c41a';
  if (score <= 0.70) return '#faad14';
  return '#ff4d4f';
}

export function getAnomalyLevel(score: number): string {
  if (score <= 0.30) return 'Normal';
  if (score <= 0.70) return 'Suspicious';
  return 'Anomalous';
}

export function formatMLScore(score: number | undefined | null): string {
  if (score === undefined || score === null || score === 0) return '-';
  return (score * 100).toFixed(0) + '%';
}

export function formatAnomalyScore(score: number | undefined | null): string {
  if (score === undefined || score === null) return '-';
  return score.toFixed(2);
}
