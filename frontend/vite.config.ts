import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // Docker コンテナ内から起動するため、ファイル変更検知にポーリングが要る
    watch: { usePolling: true },
  },
});
