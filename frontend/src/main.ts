import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import * as ElementPlusIcons from '@element-plus/icons-vue'

import App from './App.vue'
import { router } from './router'
import { currentLocale, i18n, elementLocale } from './i18n'
import { useThemeStore } from './stores/theme'
import { initViewportAdaptation } from './utils/viewport'
import './style.css'

// index.html 里写死了 lang="zh-CN"，而界面语言是按浏览器选的：不同步的话，英文界面
// 的页面在屏幕阅读器 / 翻译插件看来仍然是中文文档。
document.documentElement.lang = currentLocale

const app = createApp(App)

app.use(createPinia())
app.use(i18n)
app.use(router)
app.use(ElementPlus, { locale: elementLocale, size: 'small' })

// 主题要在挂载之前定下来：挂载后再切会让首屏闪一下浅色。初始化只是读设置 +
// 挂监听，不阻塞渲染（宿主主题是异步读的，读回来再应用一次）。
void useThemeStore().init()

// 手机上键盘顶起来时把页面缩到键盘以上（写 --kb-height，见 utils/viewport.ts）。
initViewportAdaptation()

// 图标全部注册为全局组件，模板里直接写 <el-icon><Folder /></el-icon>。
for (const [name, component] of Object.entries(ElementPlusIcons)) {
  app.component(name, component)
}

app.mount('#app')
