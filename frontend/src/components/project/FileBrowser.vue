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

      <!--
        新建：目录和文件共用一个弹窗（见 NewEntryDialog），所以工具栏也只留一个入口。
        两个图标各开一个弹窗的话，用户点「新建目录」进去还会看到「新建文件」按钮，
        反而要先想一下自己点的是哪个。
      -->
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="Plus"
        :title="t('tree.new')"
        :aria-label="t('tree.new')"
        @click="openCreate"
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

    <!--
      滚动容器接住**空白处**的右键：行 / 网格格子自己处理（下面挂 data-fs-entry），
      其余落点都算对当前目录做事（新建 / 上传 / 粘贴到这里）。不用 `.self` 判断——
      空白处的形态不止「容器本身」：最后一行下面的留白、网格的行尾、空目录的占位文字。
    -->
    <div
      class="min-h-0 flex-1 overflow-auto thin-scroll"
      @click="onBlankClick"
      @contextmenu.prevent="onBlankContext"
    >
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
          :class="[
            cutPaths.has(entryPath(entry)) ? 'opacity-50' : '',
            isSelected(entryPath(entry))
              ? 'bg-sky-50 dark:bg-sky-950/40 ring-1 ring-inset ring-sky-200 dark:ring-sky-800'
              : '',
          ]"
          data-fs-entry
          @click="onEntryClick(entry, $event)"
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
          :class="[
            cutPaths.has(entryPath(entry)) ? 'opacity-50' : '',
            isSelected(entryPath(entry))
              ? 'bg-sky-50 dark:bg-sky-950/40 ring-1 ring-inset ring-sky-200 dark:ring-sky-800'
              : '',
          ]"
          data-fs-entry
          @click="onEntryClick(entry, $event)"
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

    <NewEntryDialog v-model="newEntryOpen" :busy="creating" @submit="onCreateEntry" />

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
  Fold,
  FolderOpened,
  Grid,
  List,
  MoreFilled,
  Plus,
  Refresh,
  UploadFilled,
} from '@element-plus/icons-vue'
import type { FsEntry } from '@/api/types'
import NewEntryDialog from './NewEntryDialog.vue'
import { useUpload } from './useUpload'
import { useDownload } from './useDownload'
import { useOpenFolder } from './useOpenFolder'
import { useTabsStore } from '@/stores/tabs'
import { FILES_TAB, useWorkspaceStore } from '@/stores/workspace'
import { useFileClipboard } from './useFileClipboard'
import { dropNestedPaths, useEntrySelection } from './useEntrySelection'
import { formatTime, humanSize, isArchive } from '@/utils/format'
import { hasOpenOverlay, isTypingTarget } from '@/utils/keyboard'

const props = defineProps<{ project: string; mobile?: boolean }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const tabs = useTabsStore()
const { upload, cancel: cancelUpload, busy: uploading, percent: uploadPercent } = useUpload()
const { download } = useDownload()
const { canOpenFolder, openFolder } = useOpenFolder(() => props.project)
const { copy: copyPaths, paste: pasteAt, undo } = useFileClipboard(() => props.project)

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
const menu = reactive({ show: false, x: 0, y: 0, path: '', dir: false, blank: false, multi: false })
const menuEl = ref<HTMLElement | null>(null)

/** 剪切中的那些项淡一半（和文件树共用同一份剪贴板）。 */
const cutPaths = computed(
  () => new Set(state.value.clipboard?.cut ? state.value.clipboard.paths : []),
)

/**
 * 条目在当前目录里的完整路径：列表、网格、菜单都用它。
 *
 * 多选也按这个路径记：目录刷新只是换了一份 entries，路径本身还是同一套写法。
 */
const entryPath = (entry: FsEntry) => `${state.value.browserDir}/${entry.name}`

const {
  selected,
  isSelected,
  clear: clearSelection,
  click: selectClick,
  context: contextSelect,
  prune: pruneSelection,
} = useEntrySelection(() => entries.value.map((entry) => entryPath(entry)))

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

/** 单击：带 Ctrl / Cmd / Shift 时只改选择，不打开（多选时不该开出一堆文件标签）。 */
function onEntryClick(entry: FsEntry, event: MouseEvent) {
  selectClick(entryPath(entry), event, () => open(entry))
}

/** 点空白处取消选择（行 / 格子上的点击有自己的处理，先挡掉）。 */
function onBlankClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  if (target?.closest('[data-fs-entry]')) return
  clearSelection()
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
  () => {
    clearSelection()
    void workspace.loadBrowserDir(props.project)
  },
  { immediate: true },
)

// 目录刷新后丢掉已经不在列表里的选择（删除、外部改动都会走到这里）。
watch(entries, () => pruneSelection())

// ---- 新建 / 上传 ----

/** 「新建」弹窗的显隐（目录与文件共用一个弹窗，见 NewEntryDialog）。 */
const newEntryOpen = ref(false)
/** 正在建：弹窗按钮先禁掉，免得连点两下建出两个同名文件。 */
const creating = ref(false)

/** 工具栏按钮和空白处右键菜单共用这一个入口（菜单项点完先把菜单收掉）。 */
function openCreate() {
  hideMenu()
  newEntryOpen.value = true
}

/**
 * 弹窗提交：两个按钮只差「建目录还是建文件」，校验与后续动作完全一样。
 *
 * 名字里不能带分隔符——那会让文件被建到子目录里，和用户看着的「在这个目录里新建」
 * 对不上；空名字也在这里挡住。校验不过 / 后端报错（重名、读不到目录）时**弹窗不关**，
 * 改个名字再点一下就行。
 */
async function onCreateEntry(kind: 'file' | 'dir', raw: string) {
  if (creating.value) return
  const name = raw.trim()
  if (!name) {
    ElMessage.warning(t('tree.namePrompt'))
    return
  }
  if (/[\\/]/.test(name)) {
    ElMessage.warning(t('tree.renameInvalid'))
    return
  }
  creating.value = true
  try {
    await workspace.createEntryAt(props.project, state.value.browserDir, name, kind === 'dir')
    newEntryOpen.value = false
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    creating.value = false
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

// ---- 右键菜单 ----

const MENU_MARGIN = 8

function showMenu(event: MouseEvent, path: string, dir: boolean, blank: boolean, multi = false) {
  menu.show = true
  menu.x = event.clientX
  menu.y = event.clientY
  menu.path = path
  menu.dir = dir
  menu.blank = blank
  menu.multi = multi
  void nextTick(clampMenu)
  window.setTimeout(() => window.addEventListener('click', hideMenu, { once: true }), 0)
}

/** 行 / 网格格子上的右键：点在已选中的一项上时菜单对整批选择做事。 */
function openMenu(entry: FsEntry, event: MouseEvent) {
  const path = entryPath(entry)
  contextSelect(path)
  showMenu(event, path, entry.dir, false, selected.value.length > 1)
}

/**
 * 空白处右键：菜单对**当前目录**做事（新建 / 上传 / 粘贴到这里）。
 *
 * 判据是「落点不在任何一行里」而不是 `.self`：空白处可能是最后一行下面的留白、
 * 网格的行尾，或者这个目录本身就是空的——那样一个满宽的元素都不是，`.self` 只认
 * 容器自己被点中的那一种。
 */
function onBlankContext(event: MouseEvent) {
  // 触屏长按也会触发 contextmenu，但这个菜单是桌面右键的语义；移动端的行菜单另有入口
  if (props.mobile) return
  const target = event.target as HTMLElement | null
  if (target?.closest('[data-fs-entry]')) return
  showMenu(event, state.value.browserDir, true, true)
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
  const destDir = menuDir.value
  hideMenu()
  await pasteAt(destDir)
}

/** 复制 / 剪切选中的那一批（菜单和键盘共用）。 */
function copySelection(cut: boolean) {
  copyPaths(selected.value, cut)
}

/** Ctrl / Cmd + X / C / V / Z：只处理文件管理器的快捷键。 */
function onKeydown(event: KeyboardEvent) {
  if (tabs.activeProject !== props.project) return
  if (props.mobile || state.value.viewMode !== 'tabs' || state.value.activeTab !== FILES_TAB) return
  if (hasOpenOverlay()) return
  if (!(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey) return
  if (isTypingTarget(event)) return
  const key = event.key.toLowerCase()
  if (key !== 'c' && key !== 'x' && key !== 'v' && key !== 'z') return
  event.preventDefault()
  if (key === 'c') copySelection(false)
  else if (key === 'x') copySelection(true)
  else if (key === 'v') void pasteAt(state.value.browserDir)
  else void undo()
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

/**
 * 删除选中的那一批（Ctrl / Shift 多选后从右键菜单进入）。
 *
 * 先把「父目录 + 它里面的子项」收敛掉（见 `dropNestedPaths`）：否则删完父目录，
 * 后面那条子路径已经不存在了。确认只弹一次——逐条问是让人不敢动手的那种交互。
 */
async function removeSelected() {
  const paths = dropNestedPaths(selected.value)
  if (!paths.length) return
  hideMenu()
  try {
    await ElMessageBox.confirm(t('tree.confirmDeleteMany', { count: paths.length }), t('common.tip'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  const dirOf = (path: string) =>
    entries.value.some((entry) => entryPath(entry) === path && entry.dir)
  clearSelection()
  for (const path of paths) {
    try {
      await workspace.removeEntryAt(props.project, path, dirOf(path))
      workspace.clearClipboard(props.project, path)
    } catch (e) {
      ElMessage.error(e instanceof Error ? e.message : String(e))
    }
  }
}

type MenuItem = { label: string; run: () => void; danger?: boolean; disabled?: boolean }

const menuItems = computed<MenuItem[]>(() => {
  // 右键落在多选中的某一项上：这一份菜单对整批选择做事，列出单个文件的重命名 /
  // 下载没有意义（总不能把十项都重命名成同一个名字）。
  if (!menu.blank && menu.multi) {
    return [
      {
        label: t('tree.copy'),
        run: () => {
          hideMenu()
          copySelection(false)
        },
      },
      {
        label: t('tree.cut'),
        run: () => {
          hideMenu()
          copySelection(true)
        },
      },
      {
        label: t('common.delete'),
        danger: true,
        run: () => void removeSelected(),
      },
      {
        label: t('tree.clearSelection'),
        run: () => {
          hideMenu()
          clearSelection()
        },
      },
    ]
  }

  // 空白处：这个菜单是对**当前目录**做事的，只有新建 / 上传 / 粘贴三项。
  // 粘贴只在剪贴板里有东西时出现（复制 / 剪切在行菜单里），能不能粘由目录判。
  if (menu.blank) {
    const items: MenuItem[] = [
      { label: t('tree.new'), run: openCreate },
      { label: t('tree.upload'), run: () => pickFiles() },
    ]
    if (state.value.clipboard) {
      items.push({ label: t('tree.paste'), disabled: !canPaste.value, run: () => void paste() })
    }
    return items
  }

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
  items.push({ label: t('tree.copy'), run: () => copySelection(false) })
  items.push({ label: t('tree.cut'), run: () => copySelection(true) })
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
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  hideMenu()
  window.removeEventListener('keydown', onKeydown)
})
</script>
