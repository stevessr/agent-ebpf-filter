export interface ClusterNodeInfo {
  id: string;
  name: string;
  url: string;
  role: "master" | "slave";
  status: string;
  lastSeen: string;
  isLocal: boolean;
  version?: string;
}
export interface ClusterStateResponse {
  role: "master" | "slave";
  masterUrl: string;
  nodeUrl: string;
  nodeId: string;
  nodeName: string;
  accountConfigured: boolean;
  passwordConfigured: boolean;
  localNode: ClusterNodeInfo;
}
