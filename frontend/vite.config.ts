import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // The same bundle is loaded by Wails and Android's file:// WebView.
  // Relative asset URLs keep both hosts working without a web-server root.
  base: './',
  build: { outDir: 'dist' },
})
