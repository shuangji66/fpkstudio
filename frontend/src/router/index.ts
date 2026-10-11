import { createRouter, createWebHashHistory } from 'vue-router'
import HomeTab from '@/views/HomeTab.vue'
import ProjectTab from '@/views/ProjectTab.vue'

/**
 * 路由只描述「当前激活哪个标签」，不负责渲染标签内容。
 *
 * 标签状态由 stores/tabs.ts 持有并作为唯一真相；AppShell 同时挂载所有已打开
 * 的标签（v-show 切换），因此切换标签不会丢掉编辑中的内容。路由的作用是让
 * 地址栏可分享、可刷新、可前进后退。
 */
const routes = [
  { path: '/', name: 'home', component: HomeTab },
  { path: '/project/:name', name: 'project', component: ProjectTab, props: true },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

/** 把当前激活的标签投影到地址栏。 */
export function projectPath(name: string): string {
  return `/project/${encodeURIComponent(name)}`
}
