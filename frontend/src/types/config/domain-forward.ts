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

export interface DomainPrivacySettings {
  enabled: boolean;
  action: "warn" | "block" | "redact";
  restoreResponse: boolean;
  maxBodyBytes: number;
  categories: Array<"secret" | "email" | "phone">;
  allowlist: string[];
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
  privacy: DomainPrivacySettings;
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
  privacyEnabled?: boolean;
  privacyAction?: string;
  inferenceEnabled?: boolean;
  inferenceReady?: boolean;
  errors?: string[];
  updatedAt: string;
}
