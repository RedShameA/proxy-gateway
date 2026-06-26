import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const apiProxyTarget = 'http://localhost:8066';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
      },
    },
  },
});
