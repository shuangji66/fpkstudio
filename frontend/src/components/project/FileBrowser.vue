<template>
  <div class="flex h-full min-h-0 flex-col">
    <!--
      工具栏：一个文件管理器该有的东西都在这儿。窄面板放不下一行时会自己折行，
      按钮都只有图标（文字说明挂在原生 title 上），所以折行也不会把某一组拆散。
    -->
    <div class="flex shrink-0 flex-wrap items-center gap-0 border-b border-slate-200 dark:border-slate-700 px-1 py-0.5">
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="ArrowLeft"
        :disabled="!state.browserBack.length"
        :title="t('files.back')"
        :aria-label="t('files.back')"
        @click="workspace.browseBack(project)"
      />
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="ArrowRight"
        :disabled="!state.browserForward.length"
        :title="t('files.forward')"
        :aria-label="t('files.forward')"
        @click="workspace.browseForward(project)"
      />
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="Refresh"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="refresh"
      />

      <span class="mx-1 h-4 w-px shrink-0 bg-slate-200 dark:bg-slate-700" />

      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="DocumentAdd"
        :title="t('tree.newFile')"
        :aria-label="t('tree.newFile')"
        @click="create(false)"
      />
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="FolderAdd"
        :title="t('tree.newDir')"
        :aria-label="t('tree.newDir')"
        @click="create(true)"
      />
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="UploadFilled"
        :title="t('tree.upload')"
        :aria-label="t('tree.upload')"
        @click="pickFiles"
      />

      <span class="mx-1 h-4 w-px shrink-0 bg-slate-200 dark:bg-slate-700" />

      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="state.browserView === 'list' ? Grid : List"
        :title="state.browserView === 'list' ? t('files.grid') : t('files.list')"
        :aria-label="state.browserView === 'list' ? t('files.grid') : t('files.list')"
        @click="workspace.setBrowserView(project, state.browserView === 'list' ? 'grid' : 'list')"
      />
      <!-- 移动端固定就是这一种形态，没有左栏可以切回去 -->
      <el-button
        v-if="!mobile"
        text
        size="default"
        class="tree-tool"
        :icon="Fold"
        :title="t('files.switchToTree')"
        :aria-label="t('files.switchToTree')"
        @click="toTree"
      />

      <span class="mx-1 h-4 w-px shrink-0 bg-slate-200 dark:bg-slate-700" />

      <!--
        在飞牛的文件管理器里打开项目目录：和上面那批「对文件做什么」不是一类动作，
        单独隔一组。宿主能力，独立浏览器页面里置灰并说明原因（用原生 title）。
      -->
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="FolderOpened"
        :disabled="!canOpenFolder"
        :title="canOpenFolder ? t('common.openFolder') : t('trim.error.hostUnavailable')"
        :aria-label="t('common.openFolder')"
        @click="openFolder"
      />
    </div>

    <!-- 路径：点哪一段就去哪一层，省得只靠「后退」一层层退 -->
    <div class="hscroll thin-scroll flex shrink-0 items-center px-1.5 py-0.5 text-sm text-slate-500 dark:text-slate-400">
      <template v-for="(seg, i) in crumbs" :key="seg.path">
        <span v-if="i" class="shrink-0 px-0.5 text-slate-300 dark:text-slate-600">/</span>
        <button
          type="button"
          class="max-w-[12rem] shrink-0 truncate rounded px-1 py-0.5 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-slate-700 dark:hover:text-slate-200"
          :class="i === crumbs.length - 1 ? 'font-medium text-slate-700 dark:text-slate-200' : ''"
          :title="seg.path"
          @click="go(seg.path)"
        >
          {{ seg.label }}
        </button>
      </template>
    </div>

    <!-- 上传中：进度 + 取消（取消 = abort，后端也会跟着停，不留半截文件） -->
    <div
      v-if="uploading"
      class="flex shrink-0 items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-2 py-1"
    >
      <el-progress
        class="min-w-0 flex-1"
        :percentage="uploadPercent"
        :stroke-width="6"
        :show-text="false"
      />
      <span class="mono shrink-0 text-[11px] text-slate-500 dark:text-slate-400">{{ uploadPercent }}%</span>
      <el-button text size="default" class="tree-tool shrink-0" @click="cancelUpload">
        {{ t('common.cancel') }}
      </el-button>
    </div>

    <!-- 列表的标题栏：目录不显示大小，所以大小那一格留空 -->
    <div
      v-if="state.browserView === 'list'"
      class="flex shrink-0 items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-2 py-1 text-sm font-medium text-slate-500 dark:text-slate-400"
    >
      <span class="min-w-0 flex-1">{{ t('files.name') }}</span>
      <span class="w-16 shrink-0 text-right">{{ t('files.size') }}</span>
      <span class="w-24 shrink-0 text-right">{{ t('files.modified') }}</span>
      <span class="w-7 shrink-0" />
    </div>

    <div class="min-h-0 flex-1 overflow-auto thin-scroll">
      <div
        v-if="!entries.length && state.error"
        class="px-3 py-4 text-sm"
      >
        <p class="leading-relaxed break-all text-red-600 dark:text-red-400">{{ state.error }}</p>
        <el-button text size="default" class="mt-1" :icon="Refresh" @click="refresh">
          {{ t('common.retry') }}
        </el-button>
      </div>

      <div
        v-else-if="!entries.length"
        class="px-3 py-4 text-center text-sm text-slate-400 dark:text-slate-500"
      >
        {{ loaded ? t('tree.emptyDir') : t('common.loading') }}
      </div>

      <template v-else-if="state.browserView === 'list'">
        <div
          v-for="entry in entries"
          :key="entry.name"
          class="group flex cursor-pointer items-center gap-2 border-b border-slate-100 px-2 py-1 text-sm hover:bg-slate-100 dark:border-slate-700/60 dark:hover:bg-slate-700"
          :class="cutPath === entryPath(entry) ? 'opacity-50' : ''"
          @click="open(entry)"
          @contextmenu.prevent="openMenu(entry, $event)"
        >
          <el-icon class="shrink-0" :class="iconClass(entry)">
            <FolderOpened v-if="entry.dir" />
            <Document v-else />
          </el-icon>
          <span class="min-w-0 flex-1 truncate" :title="entry.name">{{ entry.name }}</span>
          <span class="w-16 shrink-0 text-right text-sm text-slate-400 dark:text-slate-500">
            {{ entry.dir || entry.size < 0 ? '' : humanSize(entry.size) }}
          </span>
          <span class="w-24 shrink-0 text-right text-sm text-slate-400 dark:text-slate-500">
            {{ formatTime(entry.modTime) }}
          </span>
          <el-icon
            class="w-7 shrink-0 cursor-pointer rounded p-1 text-lg text-slate-300 transition-colors hover:bg-slate-200 hover:text-slate-700 dark:text-slate-600 dark:hover:bg-slate-600 dark:hover:text-slate-200"
            :title="t('tree.more')"
            @click.stop="openMenu(entry, $event)"
          >
            <MoreFilled />
          </el-icon>
        </div>
      </template>

      <!-- 网格：没有标题栏，名字下面就是时间 -->
      <div
        v-else
        class="grid gap-1 p-2 grid-cols-[repeat(auto-fill,minmax(6.5rem,1fr))]"
      >
        <div
          v-for="entry in entries"
          :key="entry.name"
          class="relative flex cursor-pointer flex-col items-center gap-1 rounded-md border border-transparent px-2 py-2 text-center hover:border-slate-200 hover:bg-slate-100 dark:hover:border-slate-600 dark:hover:bg-slate-700"
          :class="cutPath === entryPath(entry) ? 'opacity-50' : ''"
          @click="open(entry)"
          @contextmenu.prevent="openMenu(entry, $event)"
        >
          <el-icon class="text-2xl" :class="iconClass(entry)">
            <FolderOpened v-if="entry.dir" />
            <Document v-else />
          </el-icon>
          <span class="line-clamp-2 w-full break-all text-sm leading-tight" :title="entry.name">
            {{ entry.name }}
          </span>
          <span class="text-sm text-slate-400 dark:text-slate-500">
            {{ entry.dir ? '' : humanSize(entry.size) }}
          </span>
          <!-- 触屏上没有 hover，这个按钮得一直看得见，只是颜色压得很淡 -->
          <el-icon
            class="absolute right-0.5 top-0.5 cursor-pointer rounded p-1 text-xl text-slate-300 transition-colors hover:bg-slate-200 hover:text-slate-700 dark:text-slate-600 dark:hover:bg-slate-600 dark:hover:text-slate-200"
            :title="t('tree.more')"
            @click.stop="openMenu(entry, $event)"
          >
            <MoreFilled />
          </el-icon>
        </div>
      </div>
    </div>

    <input ref="fileInput" type="file" multiple class="hidden" @change="onFiles" />

    <div
      v-if="menu.show"
      ref="menuEl"
      class="fixed z-50 min-w-[9rem] rounded-md border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 py-1 text-sm shadow-lg"
      :style="{ left: `${menu.x}px`, top: `${menu.y}px` }"
    >
      <button
        v-for="item in menuItems"
        :key="item.label"
        type="button"
        class="block w-full whitespace-nowrap px-3 py-1 text-left hover:bg-slate-100 dark:hover:bg-slate-700 disabled:cursor-not-allowed disabled:text-slate-300 dark:disabled:text-slate-600 disabled:hover:bg-transparent"
        :class="item.danger ? 'text-red-600 dark:text-red-400' : ''"
        :disabled="item.disabled === true"
        @click="item.run()"
      >
        {{ item.label }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ArrowLeft,
  ArrowRight,
  Document,
  DocumentAdd,
  Fold,
  FolderAdd,
  FolderOpened,
  Grid,
  List,
  MoreFilled,
  Refresh,
  UploadFilled,
} from '@element-plus/icons-vue'
import type { FsEntry } from '@/api/types'
import { useUpload } from './useUpload'
import { useDownload } from './useDownload'
import { useOpenFolder } from './useOpenFolder'
import { useWorkspaceStore } from '@/stores/workspace'
import { formatTime, humanSize, isArchive } from '@/utils/format'

const props = defineProps<{ project: string; mobile?: boolean }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const { upload, cancel: cancelUpload, busy: uploading, percent: uploadPercent } = useUpload()
const { download } = useDownload()
const { canOpenFolder, openFolder } = useOpenFolder(() => props.project)

const state = computed(() => workspace.state(props.project))

/**
 * 当前目录的内容。
 *
 * `dirs` 是文件树和这里共用的缓存，所以两边看到的永远是同一份：在树上新建的文件，
 * 切到标签页模式立刻就在。
 */
const entries = computed(() => state.value.dirs[state.value.browserDir] ?? [])
/** 缓存里有这个目录才算加载过——没有就是在拉，或者拉失败了（看 state.error）。 */
const loaded = computed(() => state.value.dirs[state.value.browserDir] !== undefined)

/**
 * 面包屑：第一段是「项目根」而不是项目名。
 *
 * 项目名就写在标签上，路径里再重复一遍只会挤掉真正有用的目录名；但这一段的**跳回根目录**
 * 还是要留着，所以换成一句通用的根。
 */
const crumbs = computed(() => {
  const dir = state.value.browserDir
  const rel = dir === props.project ? '' : dir.slice(props.project.length + 1)
  const out = [{ label: t('files.root'), path: props.project }]
  let acc = props.project
  for (const part of rel ? rel.split('/') : []) {
    acc = `${acc}/${part}`
    out.push({ label: part, path: acc })
  }
  return out
})

const fileInput = ref<HTMLInputElement | null>(null)
const menu = reactive({ show: false, x: 0, y: 0, path: '', dir: false })
const menuEl = ref<HTMLElement | null>(null)

/** 剪切中的那一项淡一半（和文件树共用同一份剪贴板）。 */
const cutPath = computed(() => (state.value.clipboard?.cut ? state.value.clipboard.path : ''))

/** 条目在当前目录里的完整路径：列表、网格、菜单都用它。 */
const entryPath = (entry: FsEntry) => `${state.value.browserDir}/${entry.name}`

function iconClass(entry: FsEntry): string {
  return entry.dir
    ? 'text-amber-500 dark:text-amber-400'
    : 'text-slate-400 dark:text-slate-500'
}

// ---- 导航 ----

/** 进目录 / 开文件：两种排布下的单击都是这个行为。 */
function open(entry: FsEntry) {
  if (entry.dir) {
    go(entryPath(entry))
    return
  }
  void workspace.openFile(props.project, entryPath(entry))
}

function go(dir: string) {
  workspace.browseTo(props.project, dir)
}

function refresh() {
  void workspace.loadBrowserDir(props.project, true)
}

function toTree() {
  workspace.setViewMode(props.project, 'tree')
}

// 切目录就拉一次内容；切回来时缓存里有就不再请求（loadDir 自己会跳过）。
watch(
  () => state.value.browserDir,
  () => void workspace.loadBrowserDir(props.project),
  { immediate: true },
)

// ---- 新建 / 上传 ----

async function create(dir: boolean) {
  let name = ''
  try {
    const res = await ElMessageBox.prompt(
      t('tree.namePrompt'),
      dir ? t('tree.newDir') : t('tree.newFile'),
      {
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel'),
        inputValidator: (value) => (value && value.trim() ? true : t('tree.namePrompt')),
      },
    )
    name = res.value.trim()
  } catch {
    return
  }
  try {
    await workspace.createEntryAt(props.project, state.value.browserDir, name, dir)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

function pickFiles() {
  fileInput.value?.click()
}

async function onFiles(event: Event) {
  const files = Array.from((event.target as HTMLInputElement).files ?? [])
  try {
    if (files.length) await upload(props.project, state.value.browserDir, files)
  } finally {
    if (fileInput.value) fileInput.value.value = ''
  }
}

// ---- 行内「···」菜单 ----

const MENU_MARGIN = 8

function openMenu(entry: FsEntry, event: MouseEvent) {
  menu.show = true
  menu.x = event.clientX
  menu.y = event.clientY
  menu.path = entryPath(entry)
  menu.dir = entry.dir
  void nextTick(clampMenu)
  window.setTimeout(() => window.addEventListener('click', hideMenu, { once: true }), 0)
}

function clampMenu() {
  const el = menuEl.value
  if (!el) return
  const { width, height } = el.getBoundingClientRect()
  menu.x = Math.max(MENU_MARGIN, Math.min(menu.x, window.innerWidth - width - MENU_MARGIN))
  menu.y = Math.max(MENU_MARGIN, Math.min(menu.y, window.innerHeight - height - MENU_MARGIN))
}

function hideMenu() {
  menu.show = false
}

/** 菜单指向的目录：目录就是它自己，文件取父目录（粘贴要用）。 */
const menuDir = computed(() =>
  menu.dir ? menu.path : menu.path.slice(0, menu.path.lastIndexOf('/')),
)

const canPaste = computed(() => workspace.canPasteTo(props.project, menuDir.value))

async function paste() {
  const clip = state.value.clipboard
  if (!clip) return
  const destDir = menuDir.value
  const name = clip.path.split('/').pop() ?? ''
  hideMenu()
  try {
    const outcome = await workspace.pasteInto(props.project, destDir)
    if (outcome === 'exists') ElMessage.warning(t('tree.pasteExists', { name }))
    else if (outcome === 'same') ElMessage.info(t('tree.pasteSame', { name }))
    else if (outcome === 'unreadable') {
      ElMessage.error(t('tree.pasteListFailed', { name: destDir.split('/').pop() ?? destDir }))
    }
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

async function extract() {
  const path = menu.path
  const name = path.split('/').pop() ?? path
  hideMenu()
  try {
    const res = await workspace.extractAt(props.project, path)
    if (res.skipped > 0) {
      ElMessage.warning(t('tree.extractSkipped', { name, files: res.files, skipped: res.skipped }))
    } else {
      ElMessage.success(t('tree.extractDone', { name, files: res.files }))
    }
    // 归档里的符号链接与 FIFO / 设备节点都不会还原，单独说一句
    // （不是错误，但用户得知道少了什么）。
    if (res.links > 0) ElMessage.warning(t('tree.extractLinks', { count: res.links }))
    if (res.specials > 0) ElMessage.warning(t('tree.extractSpecials', { count: res.specials }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

/**
 * 重命名当前项。
 *
 * 文件树里是行内输入框，这里是列表 / 网格，没有「正在编辑的那一行」可用，所以走
 * 弹窗。真正的重名判断在 store 里（`renameEntryAt`），这里只挡住空名字和带分隔符的
 * 名字——后者会让文件被搬进子目录，不是用户按「重命名」时想要的。
 */
async function rename() {
  const path = menu.path
  const name = path.split('/').pop() ?? path
  hideMenu()
  let next = ''
  try {
    const res = await ElMessageBox.prompt(t('tree.namePrompt'), t('common.rename'), {
      inputValue: name,
      inputValidator: (value) =>
        value && value.trim() && !/[\\/]/.test(value.trim()) ? true : t('tree.renameInvalid'),
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
    })
    next = String(res.value ?? '').trim()
  } catch {
    return
  }
  if (!next || next === name) return
  const parent = path.slice(0, path.lastIndexOf('/'))
  try {
    await workspace.renameEntryAt(props.project, path, `${parent}/${next}`)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

async function remove() {
  const path = menu.path
  const dir = menu.dir
  const name = path.split('/').pop() ?? path
  hideMenu()
  try {
    await ElMessageBox.confirm(t('tree.confirmDelete', { name }), t('common.tip'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    await workspace.removeEntryAt(props.project, path, dir)
    workspace.clearClipboard(props.project, path)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

type MenuItem = { label: string; run: () => void; danger?: boolean; disabled?: boolean }

const menuItems = computed<MenuItem[]>(() => {
  const path = menu.path
  const name = path.split('/').pop() ?? path
  const items: MenuItem[] = []

  // 有剪贴板内容时多给一条「粘贴」：不然在标签页模式里点了「复制 / 剪切」就再也
  // 没有地方能把它粘下去（文件树的菜单够不着这里）。
  if (state.value.clipboard) {
    items.push({ label: t('tree.paste'), disabled: !canPaste.value, run: () => void paste() })
  }
  // 重命名在文件树里是行内输入框，这里是列表 / 网格、没有「正在编辑的那一行」可用，
  // 所以走弹窗。少了这一条的话，标签页模式下用户根本改不了文件名。
  items.push({ label: t('common.rename'), run: () => void rename() })
  items.push({
    label: t('tree.copy'),
    run: () => {
      workspace.copyToClipboard(props.project, path, false)
      ElMessage.success(t('tree.copied', { name }))
    },
  })
  items.push({
    label: t('tree.cut'),
    run: () => workspace.copyToClipboard(props.project, path, true),
  })
  items.push({
    label: t('common.delete'),
    danger: true,
    run: () => void remove(),
  })
  // 目录是后端现打的 zip（`zip: true` 挂一条「正在打包」的提示），文件是直传
  items.push({ label: t('common.download'), run: () => void download(path, { zip: menu.dir }) })
  if (!menu.dir && isArchive(name)) {
    items.push({ label: t('tree.extract'), run: () => void extract() })
  }
  return items
})

onMounted(() => {
  void workspace.ensure(props.project)
})

onUnmounted(hideMenu)
</script>
