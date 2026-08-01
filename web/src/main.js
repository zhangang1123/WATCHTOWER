import { createApp } from 'vue'
import 'element-plus/dist/index.css'
import App from './App.vue'
import './style.css'

try {
  const savedTheme = window.localStorage.getItem('watchtower-theme')
  const systemTheme = window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
  document.documentElement.dataset.theme = ['light', 'dark'].includes(savedTheme) ? savedTheme : systemTheme
} catch {
  document.documentElement.dataset.theme = 'dark'
}

const app = createApp(App)

app.mount('#app')
