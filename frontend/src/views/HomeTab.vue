<template>
  <div class="flex h-full min-h-0 flex-col">
    <!--
      工具条（搜索 + 按钮 + 视图切换）和新建 / 导入面板固定在顶部，不参与下面的滚动：
      项目多的时候工具条跟着滚出去就没法用了。
    -->
    <div class="shrink-0 border-b border-slate-200 dark:border-slate-700 px-4 py-3">
      <!-- 项目计数不在这儿：它和称号一起在项目列表最后一行下面（见下面的 footer） -->
      <div class="home-actions flex flex-wrap items-center gap-3">
        <!--
          搜索框按内容自适应宽度（`.home-search`，见 style.css）：placeholder / 关键字
          长了就跟着变宽，和旁边那排「图标 + 文字」的按钮一样不写死像素。
          工具条只用父级的 gap 排间距，按钮之间不再叠 Element Plus 的相邻外边距
          （`.home-actions` 那条规则），所有间隔和「搜索 → 新建」保持一致。
        -->
        <el-input
          v-model="projects.keyword"
          class="home-search"
          size="default"
          clearable
          :placeholder="t('home.searchPlaceholder')"
          :prefix-icon="Search"
        />
        <el-button
          size="default"
          :icon="Refresh"
          :loading="projects.loading"
          @click="projects.refresh()"
        >
          {{ t('common.refresh') }}
        </el-button>
        <el-button
          size="default"
          :type="panel === 'create' ? 'primary' : 'default'"
          :icon="Plus"
          @click="toggle('create')"
        >
          {{ t('home.newProject') }}
        </el-button>
        <el-button
          size="default"
          :type="panel === 'import' ? 'primary' : 'default'"
          :icon="Upload"
          @click="toggle('import')"
        >
          {{ t('home.importFpk') }}
        </el-button>
        <!-- 图标制作：不依赖项目，和「新建 / 导入」并排 -->
        <el-button size="default" :icon="Picture" @click="iconOpen = true">
          {{ t('iconTab.title') }}
        </el-button>

        <span class="flex-1" />

        <!--
          显示切换只有一个按钮：字面是**当前**的视图，点一下换过去（和编辑器的
          表单 / 源码切换同一套）。平铺 = 卡片按宽度平铺、整页滚动；分页 = 一页铺满，
          底部翻页，给项目很多的时候用。
        -->
        <el-button
          size="default"
          :icon="view === 'tile' ? Grid : Menu"
          :title="t('home.viewSwitch')"
          @click="toggleView"
        >
          {{ view === 'tile' ? t('home.viewTile') : t('home.viewPaged') }}
        </el-button>

        <!-- 设置：主题 + 项目根目录 / fpk 输出目录 -->
        <el-button size="default" :icon="Setting" @click="settingsOpen = true">
          {{ t('settings.title') }}
        </el-button>
      </div>

      <!--
        分页的换页栏钉在**顶部**（项目区自己滚，它永远在），只有一页时不显示——
        那会儿它只是个噪声。翻页按钮的尺寸跟着容器走，所以窗口一变，每页几个、
        共几页都会跟着重算。
      -->
      <div
        v-if="view === 'paged' && pageCount > 1"
        class="mt-3 flex items-center justify-center"
      >
        <el-pagination
          size="default"
          layout="prev, pager, next"
          :total="projects.filtered.length"
          :page-size="fit.perPage"
          :current-page="page"
          @current-change="onPageChange"
        />
      </div>

      <CreateProjectForm
        v-if="panel === 'create'"
        class="mt-3"
        @done="onCreated"
        @cancel="panel = ''"
      />
      <ImportFpkForm
        v-else-if="panel === 'import'"
        class="mt-3"
        @done="onImported"
        @cancel="panel = ''"
      />
    </div>

    <div ref="stage" class="min-h-0 flex-1 overflow-auto thin-scroll p-4">
      <!--
        分页视图这里是个 flex 列：卡片区 `flex-1` 撑满（网格行高按它算），
        下面的计数行不跟它抢高度；平铺视图就是普通的块级流，卡片多了整页往下滚。
      -->
      <div :class="view === 'paged' ? 'flex h-full min-h-0 flex-col' : ''">
        <ProjectGrid
          :items="visible"
          :fit="view === 'paged' ? fit : null"
          class="min-h-0 flex-1"
          @open="openProject"
        />

        <!--
          项目计数和称号：跟在最后一个项目**下一行**居中，上面一条分隔线。
          以前它在顶栏——那位置更像"筛选结果"，放到底部才是"一共这么多"。
        -->
        <footer
          v-if="projects.count"
          ref="footEl"
          class="mt-4 shrink-0 border-t border-slate-200 pt-3 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400"
        >
          {{ t('home.subtitle', { count: projects.count }) }}
          <span class="mx-1 text-slate-300 dark:text-slate-600">·</span>
          {{ t(rankKey) }}
        </footer>
      </div>
    </div>

    <IconDialog v-model="iconOpen" />
    <SettingsDialog v-model="settingsOpen" @saved="onSettingsSaved" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Grid, Menu, Picture, Plus, Refresh, Search, Setting, Upload } from '@element-plus/icons-vue'
import CreateProjectForm from '@/components/home/CreateProjectForm.vue'
import IconDialog from '@/components/home/IconDialog.vue'
import ImportFpkForm from '@/components/home/ImportFpkForm.vue'
import ProjectGrid from '@/components/home/ProjectGrid.vue'
import SettingsDialog from '@/components/home/SettingsDialog.vue'
import { useProjectsStore } from '@/stores/projects'
import { useTabsStore } from '@/stores/tabs'
import { useWorkspaceStore } from '@/stores/workspace'
import { projectLabel } from '@/utils/format'
import { clampPage, fitGrid, rankKeyFor } from '@/utils/paging'
import type { ProjectInfo } from '@/api/types'

const { t } = useI18n()
const projects = useProjectsStore()
const tabs = useTabsStore()
const workspace = useWorkspaceStore()

const panel = ref<'create' | 'import' | ''>('')
const settingsOpen = ref(false)
const iconOpen = ref(false)

/**
 * 显示方式：`tile` 平铺（卡片铺满宽度、整页滚动）/ `paged` 分页（一页铺满、底部翻页）。
 * 和侧栏宽度一样记在本地；老版本存的 `grid` / `list` 都当成平铺（列表视图已经去掉了）。
 */
type ViewMode = 'tile' | 'paged'
const storedView = window.localStorage.getItem('fpkstudio.homeView')
const view = ref<ViewMode>(storedView === 'paged' ? 'paged' : 'tile')

watch(view, (next) => window.localStorage.setItem('fpkstudio.homeView', next))

function toggleView() {
  view.value = view.value === 'tile' ? 'paged' : 'tile'
}

// ---- 分页：每页几个跟着容器尺寸走 ----

const stage = ref<HTMLElement | null>(null)
const stageSize = ref({ w: 0, h: 0, foot: 0 })
const page = ref(1)
const footEl = ref<HTMLElement | null>(null)

/** 项目计数行的占位高度（含它的 `mt-4`）：分页时要把它从可用高度里扣掉。 */
const footReserve = computed(() => (stageSize.value.foot ? stageSize.value.foot + 16 : 0))

/**
 * 容器可用尺寸 → 一页放几个。
 *
 * 两笔要自己扣掉：`p-4` 的内边距（量到的是容器外框，卡片能用的只有内容区），
 * 以及底部那行计数（它在项目区里面，分页的一页要连它一起放下）。
 * 尺寸为 0（还没挂上 / 被隐藏）时 fitGrid 会给 1×1 的兜底，不会算出空页。
 */
const fit = computed(() => {
  const pad = 32
  const reserve = view.value === 'paged' ? footReserve.value : 0
  return fitGrid(stageSize.value.w - pad, stageSize.value.h - pad - reserve)
})

/** 按项目数量给的称号（每多一档换一个，见 utils/paging.ts）。 */
const rankKey = computed(() => rankKeyFor(projects.count))

/** 共几页（至少 1，页面上要显示「1」）。 */
const pageCount = computed(() => Math.max(1, Math.ceil(projects.filtered.length / fit.value.perPage)))

/** 平铺 = 全部；分页 = 当前页那一段。 */
const visible = computed(() => {
  if (view.value === 'tile') return projects.filtered
  const start = (page.value - 1) * fit.value.perPage
  return projects.filtered.slice(start, start + fit.value.perPage)
})

function onPageChange(next: number) {
  page.value = next
}

/**
 * 每页几个变了（窗口缩放 / 换了视图）就重新钳一次页码：变小之后原页码可能越界，
 * 这时退到最后一页而不是第一页——用户多半是在看后面几页时缩的窗口。
 */
watch([() => fit.value.perPage, () => projects.filtered.length], () => {
  page.value = clampPage(page.value, projects.filtered.length, fit.value.perPage)
})

/** 换关键字等于换了一批项目，回到第一页。 */
watch(() => projects.keyword, () => {
  page.value = 1
})

/**
 * 量一次项目区（容器尺寸 + 计数行高度）。
 *
 * 计数行的高度必须和容器一起量：它在项目区**里面**，分页时要从可用高度里扣掉，
 * 而它的高度会跟着文案（共 N 个项目 · 称号）换行变。容器尺寸由 ResizeObserver 推，
 * 项目数变化（列表从空到有、称号换档）时没有 resize 事件，得自己补一次。
 */
function measureStage() {
  const el = stage.value
  if (!el) return
  stageSize.value = { w: el.clientWidth, h: el.clientHeight, foot: footEl.value?.offsetHeight ?? 0 }
}

let observer: ResizeObserver | undefined

onMounted(() => {
  measureStage()
  if (!stage.value || typeof ResizeObserver === 'undefined') return
  observer = new ResizeObserver(measureStage)
  observer.observe(stage.value)
})

// flush: 'post'：等 DOM 更新完再量，否则量到的还是上一轮的计数行
watch([() => projects.count, view, panel], () => void nextTick(measureStage), { flush: 'post' })

onUnmounted(() => observer?.disconnect())

function toggle(next: 'create' | 'import') {
  panel.value = panel.value === next ? '' : next
}

function openProject(info: ProjectInfo) {
  tabs.openProject(info.name, projectLabel(info))
  void workspace.ensure(info.name)
}

/**
 * 目录设置换了之后必须重新拉一次项目列表：项目列表读的就是项目根目录，
 * 还拿着旧目录的那一份，用户会以为设置没生效。
 */
function onSettingsSaved() {
  void projects.refresh()
}

function onCreated(info: ProjectInfo | null) {
  panel.value = ''
  if (info) ElMessage.success(t('home.created', { name: info.name }))
}

function onImported(info: ProjectInfo | undefined) {
  panel.value = ''
  if (info) {
    ElMessage.success(t('home.imported', { name: info.name }))
    openProject(info)
  }
}
</script>
