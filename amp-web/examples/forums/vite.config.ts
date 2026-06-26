import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// The forums SPA consumes @art-media-platform/web from the workspace root via a
// file: dependency (its built dist/).  No alias needed — standard resolution.
export default defineConfig({
  plugins: [react()],
  server: { port: 5174 },
});
