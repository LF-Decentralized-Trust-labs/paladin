import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => ({
  base: './',
  plugins: [react()],
  server: {
    port: mode === 'node3' ? 3003 : mode === 'node2' ? 3002 : 3000,
    proxy: {
      '/': {
        target:
          mode === 'node3'
            ? 'http://localhost:31748'
            : mode === 'node2'
            ? 'http://localhost:31648'
            : 'http://localhost:31548',
        secure: false,
        bypass: (req, _resolveConfig, _options) =>
          req.method === 'POST' ? undefined : req.url,
      },
    },
  },
}));
