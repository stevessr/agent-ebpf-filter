import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, URL } from "url";

// Get backend port from shared file, or fallback to 8080
let backendPort = 8080;
try {
  const portFile = path.resolve(__dirname, "../backend/.port");
  if (fs.existsSync(portFile)) {
    backendPort = parseInt(fs.readFileSync(portFile, "utf-8").trim(), 10);
  }
} catch (e) {
  // Use default
}

export const resolveBackendUrl = (override?: string, port = 8080) => {
  const configured = override?.trim().replace(/\/+$/, "");
  if (configured) return configured;
  const validPort = Number.isInteger(port) && port > 0 && port <= 65535;
  return `http://127.0.0.1:${validPort ? port : 8080}`;
};

const backendUrl = resolveBackendUrl(
  process.env.VITE_AGENT_BACKEND_URL,
  backendPort,
);
const backendWsUrl = backendUrl.replace(/^http/, "ws");
const bypassHtmlToIndex = (req: { headers: { accept?: string } }) => {
  if (req.headers.accept?.includes("text/html")) {
    return "/index.html";
  }
};

const resolveAliases: Array<{ find: string | RegExp; replacement: string }> = [
  {
    find: "@",
    replacement: fileURLToPath(new URL("./src", import.meta.url)),
  },
];

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: resolveAliases,
  },
  server: {
    proxy: {
      "/ws": {
        target: backendWsUrl,
        ws: true,
      },
      "/register": backendUrl,
      "/unregister": backendUrl,
      "/shell-sessions": backendUrl,
      "/mcp": backendUrl,
      "/cluster": backendUrl,
      "/data": backendUrl,
      // Proxy API calls but let frontend handle UI routes
      "^/config/(tags|comms|paths|prefixes|rules|runtime|access-token|export|import|hooks|ml|event-types|redaction-policy).*":
        {
          target: backendUrl,
          bypass: bypassHtmlToIndex,
        },
      "/system": backendUrl,
      "^/events(/|$)": backendUrl,
      "^/network(/|$)": {
        target: backendUrl,
        bypass: bypassHtmlToIndex,
      },
      "^/tls-capture(/|$)": {
        target: backendUrl,
        bypass: bypassHtmlToIndex,
      },
      "^/sandbox(/|$)": backendUrl,
    },
  },
});
