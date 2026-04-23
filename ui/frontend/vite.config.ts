import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  build: {
    // Resurrector ships as a Wails app running in Microsoft Edge WebView2,
    // so we can safely target modern syntax (BigInt literals, etc.).
    target: 'esnext',
  },
})
