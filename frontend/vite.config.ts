import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
      '/register': 'http://localhost:8080',
      '/unregister': 'http://localhost:8080',
      '/config': 'http://localhost:8080',
    },
  },
})
