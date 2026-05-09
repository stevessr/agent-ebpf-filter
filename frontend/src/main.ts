import { createApp } from 'vue'
import type { Plugin } from 'vue'
import App from './App.vue'
import Antd from 'ant-design-vue'
import router from './router'
import 'ant-design-vue/dist/reset.css'
import './style.css'
import axios from 'axios'
import {
  buildRequestHeaders,
} from './utils/requestContext'

const applyStoredRequestContext = () => {
  const headers = buildRequestHeaders()
  if (!headers['X-API-KEY']) return
  axios.defaults.headers.common['X-API-KEY'] = headers['X-API-KEY']
  axios.defaults.headers.common.Authorization = headers.Authorization
}

axios.interceptors.request.use((config) => {
  const headers = buildRequestHeaders()
  if (Object.keys(headers).length) {
    config.headers = config.headers ?? {}
    Object.assign(config.headers as Record<string, string>, headers)
  }
  return config
})

applyStoredRequestContext()

const app = createApp(App)
app.use(Antd as unknown as Plugin)
app.use(router as unknown as Plugin)
app.mount('#app')
