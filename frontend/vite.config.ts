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
      // Only proxy /config if it has a subpath (API calls), 
      // allowing the frontend to handle the base /config route.
      '^/config/.*': backendUrl,
      '/system': backendUrl,
    },
  },
})
