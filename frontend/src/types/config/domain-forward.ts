export interface DomainForwardRoute {
  host: string;
  upstream?: string;
  certFile?: string;
  keyFile?: string;
}
export interface DomainForwardProxySettings {
  enabled: boolean;
  httpPort: number;
  httpsPort: number;
  defaultScheme: "http" | "https";
  allowAnyHost: boolean;
  dnsResolver?: string;
  dialTimeoutSeconds: number;
  certFile?: string;
  keyFile?: string;
  routes: DomainForwardRoute[];
}
export interface DomainForwardProxyStatus {
  enabled: boolean;
  httpRunning: boolean;
  httpsRunning: boolean;
  httpAddress?: string;
  httpsAddress?: string;
  httpPort: number;
  httpsPort: number;
  routeCount: number;
  allowAnyHost: boolean;
  dnsResolver?: string;
  errors?: string[];
  updatedAt: string;
}
