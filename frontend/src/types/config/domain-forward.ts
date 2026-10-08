export interface DomainForwardRoute {
  host: string;
  upstream?: string;
  certFile?: string;
  keyFile?: string;
}

export interface DomainBodyRewriteRule {
  id?: string;
  enabled: boolean;
  direction: "request" | "response" | "both";
  host?: string;
  pathPrefix?: string;
  contentType?: string;
  find: string;
  replace: string;
}

export interface DomainModelRewriteRule {
  host?: string;
  from: string;
  to: string;
}

export interface DomainNativeInferenceSettings {
  enabled: boolean;
  modelFile?: string;
  direction: "request" | "response" | "both";
  host?: string;
  pathPrefix?: string;
  contentType?: string;
  minTokenBytes?: number;
  maxTokenBytes?: number;
}

export interface DomainBodyRewriteSettings {
  enabled: boolean;
  maxBodyBytes: number;
  rules: DomainBodyRewriteRule[];
  modelRules: DomainModelRewriteRule[];
  inference: DomainNativeInferenceSettings;
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
  tlsInterceptEnabled: boolean;
  tlsInterceptAllowlist?: string;
  tlsInterceptCaCertFile?: string;
  tlsInterceptCaKeyFile?: string;
  tlsInterceptLeafTtlSeconds?: number;
  rewrite: DomainBodyRewriteSettings;
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
  tlsInterceptEnabled?: boolean;
  rewriteEnabled?: boolean;
  inferenceEnabled?: boolean;
  inferenceReady?: boolean;
  errors?: string[];
  updatedAt: string;
}
