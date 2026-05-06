import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const controllerTarget = process.env.AGORA_CONTROLLER_URL?.trim() || 'http://localhost:18080';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/v1': {
        target: controllerTarget,
        changeOrigin: true,
      },
    },
  },
});
