import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"

export default defineConfig({
  plugins: [react()],
  build: { outDir: "../internal/web/dist", emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      "/api": process.env.EASYEMS_API || "http://127.0.0.1:62943",
    },
  },
})
