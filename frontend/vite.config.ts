import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import fs from 'node:fs'
import path from 'node:path'

// Get backend port from shared file, or fallback to 8080
let backendPort = 8080;
try {
  const portFile = path.resolve(__dirname, '../backend/.port');
  if (fs.existsSync(portFile)) {
    backendPort = parseInt(fs.readFileSync(portFile, 'utf-8').trim(), 10);
  }
} catch (e) {
  // Use default
}

const backendUrl = `http://localhost:${backendPort}`;
const backendWsUrl = `ws://localhost:${backendPort}`;

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/ws': {
        target: backendWsUrl,
        ws: true,
      },
      '/register': backendUrl,
      '/unregister': backendUrl,
      '/shell-sessions': backendUrl,
      '/mcp': backendUrl,
      '/cluster': backendUrl,
      '/data': backendUrl,
      // Proxy API calls but let frontend handle UI routes
      '^/config/(tags|comms|paths|prefixes|rules|runtime|access-token|export|import|hooks|ml|event-types).*': {
        target: backendUrl,
        bypass: (req) => {
          if (req.headers.accept?.includes('text/html')) {
            return '/index.html';
          }
        },
      },
      '/system': backendUrl,
      '^/events(/|$)': backendUrl,
      '^/network(/|$)': backendUrl,
      '^/tls-capture(/|$)': backendUrl,
      '^/sandbox(/|$)': backendUrl,
    },
  },
})
