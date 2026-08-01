import path from 'node:path'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const rootDir = path.resolve(import.meta.dirname, '..')
  const env = loadEnv(mode, rootDir, '')
  const webPort = Number(env.WATCHTOWER_WEB_PORT || 3000)
  const gatewayTarget = env.WATCHTOWER_GATEWAY_URL || `http://127.0.0.1:${env.WATCHTOWER_PORT || 8080}`
  const wsTarget = gatewayTarget.replace(/^http/, 'ws')

  return {
    envDir: rootDir,
    plugins: [vue()],
    server: {
      port: webPort,
      strictPort: true,
      proxy: {
        '/api': { target: gatewayTarget, changeOrigin: true },
        '/ws': { target: wsTarget, ws: true },
        '/webhook': { target: gatewayTarget, changeOrigin: true },
        '/healthz': { target: gatewayTarget, changeOrigin: true },
      },
    },
  }
})
