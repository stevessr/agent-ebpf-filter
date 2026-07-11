export interface KernelRiskFeedbackSettings {
  enabled: boolean;
  minRiskScore: number;
  enforceNetwork: boolean;
  enforceFileNames: boolean;
  enforceExec: boolean;
  maxActionsPerMinute: number;
}
