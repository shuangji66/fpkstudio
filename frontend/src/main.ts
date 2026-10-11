import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import * as ElementPlusIcons from '@element-plus/icons-vue'

import App from './App.vue'
import { router } from './router'
import { i18n, elementLocale } from './i18n'
import './style.css'

const app = createApp(App)

app.use(createPinia())
app.use(i18n)
app.use(router)
app.use(ElementPlus, { locale: elementLocale, size: 'small' })

// 图标全部注册为全局组件，模板里直接写 <el-icon><Folder /></el-icon>。
for (const [name, component] of Object.entries(ElementPlusIcons)) {
  app.component(name, component)
}

app.mount('#app')
