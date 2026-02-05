import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/query': {
        target: 'http://localhost:8484',
        ws: true,
      },
    },
  },
})
