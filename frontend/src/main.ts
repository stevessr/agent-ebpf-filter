import { createApp } from 'vue'
import App from './App.vue'
import Antd from 'ant-design-vue'
import router from './router'
import 'ant-design-vue/dist/reset.css'
import './style.css'
import axios from 'axios'

const API_TOKEN_STORAGE_KEY = 'agent-ebpf.apiToken'

const applyStoredApiToken = () => {
  if (typeof window === 'undefined') return
  const token = window.localStorage.getItem(API_TOKEN_STORAGE_KEY)
  if (!token) return
  axios.defaults.headers.common['X-API-KEY'] = token
  axios.defaults.headers.common.Authorization = `Bearer ${token}`
}

axios.interceptors.request.use((config) => {
  if (typeof window === 'undefined') {
    return config
  }
  const token = window.localStorage.getItem(API_TOKEN_STORAGE_KEY)
  if (token) {
    config.headers = config.headers ?? {}
    ;(config.headers as Record<string, string>)['X-API-KEY'] = token
    ;(config.headers as Record<string, string>)['Authorization'] = `Bearer ${token}`
  }
  return config
})

applyStoredApiToken()

const app = createApp(App)
app.use(Antd)
app.use(router)
app.mount('#app')
