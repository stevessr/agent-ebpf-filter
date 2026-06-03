/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_AGENT_BUILD_FEATURES?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
