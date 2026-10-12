<template>
  <div class="flex h-full min-h-0 flex-col">
    <!-- 工具栏：按钮紧凑排布，窄侧边栏（最小 180px）也放得下；再窄就换行 -->
    <div class="flex shrink-0 flex-wrap items-center gap-0 border-b border-slate-200 dark:border-slate-700 px-1 py-0.5">
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="Refresh"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        @click="refreshAll"
      />
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="allCollapsed ? Expand : Fold"
        :title="allCollapsed ? t('tree.expandAll') : t('tree.collapseAll')"
        :aria-label="allCollapsed ? t('tree.expandAll') : t('tree.collapseAll')"
        @click="allCollapsed ? expandAll() : collapseAll()"
      />
      <!-- 文件管理器的另一种形态：固定「文件」标签 + 列表 / 网格 -->
      <el-button
        text
        size="default"
        class="tree-tool"
        :icon="Switch"
        :title="t('files.switchToTabs')"
        :aria-label="t('files.switchToTabs')"
        @click="toTabs"
      />
      <!--
        拖宽左栏的手柄。分隔线本身只有 1px、还没法用手指点中，所以顶栏右侧再放一个
        吸在分隔线上的按钮：按下去就是拖动（事件抛给 ProjectTab，和拖那条线同一段逻辑）。
        touch-none 是必须的：不给的话触屏会把 pointermove 当成滚动手势收走。
      -->
      <el-button
        text
        size="default"
        class="tree-tool -mr-1 ml-auto cursor-col-resize touch-none"
        :icon="Rank"
        :title="t('editor.resizeHint')"
        :aria-label="t('editor.resizeHint')"
        @pointerdown="onResizeStart"
      />
    </div>

    <!--
      上传中：进度 + 取消。左栏很窄，所以进度条用 flex-1 撑满剩下的宽度、百分比和
      取消按钮都是定宽的小件；不这样排的话 180px 的侧边栏放不下。
    -->
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

    <div
      class="min-h-0 flex-1 overflow-auto thin-scroll py-1 text-sm"
      @click.self="selected = project"
      @contextmenu.self.prevent="openMenu(project, true, $event)"
    >
      <div
        v-if="state.loading && !rows.length"
        class="px-3 py-4 text-center text-sm text-slate-400 dark:text-slate-500"
      >
        {{ t('common.loading') }}
      </div>

      <div v-else-if="!rows.length" class="px-3 py-4 text-center text-sm">
        <template v-if="state.error">
          <p class="text-left leading-relaxed break-all text-red-600 dark:text-red-400">{{ state.error }}</p>
          <el-button text size="default" class="mt-1" :icon="Refresh" @click="refreshAll">
            {{ t('common.retry') }}
          </el-button>
        </template>
        <span v-else class="text-slate-400 dark:text-slate-500">{{ t('tree.emptyDir') }}</span>
      </div>

      <template v-for="row in rows" :key="row.path">
        <div
          class="group flex cursor-pointer items-center gap-1 rounded px-1.5 py-[3px] hover:bg-slate-100 dark:hover:bg-slate-700"
          :class="[
            selected === row.path
              ? 'bg-sky-50 dark:bg-sky-950/40 ring-1 ring-inset ring-sky-200 dark:ring-sky-800'
              : '',
            cutPath === row.path ? 'opacity-50' : '',
          ]"
          @click="onClick(row)"
          @contextmenu.prevent="openMenu(row.path, row.dir, $event)"
        >
          <span :style="{ width: `${row.depth * 12}px` }" class="shrink-0" />

          <el-icon v-if="row.dir" class="shrink-0 text-slate-400 dark:text-slate-500">
            <CaretRight v-if="!state.expanded[row.path]" />
            <CaretBottom v-else />
          </el-icon>
          <span v-else class="w-[1em] shrink-0" />

          <el-icon class="shrink-0" :class="row.dir ? 'text-amber-500 dark:text-amber-400' : 'text-slate-400 dark:text-slate-500'">
            <FolderOpened v-if="row.dir && state.expanded[row.path]" />
            <Folder v-else-if="row.dir" />
            <Document v-else />
          </el-icon>

          <template v-if="renaming === row.path">
            <input
              ref="renameInput"
              v-model="renameValue"
              class="mono min-w-0 flex-1 rounded border border-sky-400 px-1 text-sm outline-none"
              @click.stop
              @keydown.enter.stop="commitRename(row.path)"
              @keydown.esc.stop="cancelEdit"
              @blur="cancelEdit"
            />
          </template>
          <template v-else>
            <span class="min-w-0 flex-1 truncate" :title="row.path">{{ row.name }}</span>
          </template>

          <!-- 和右键菜单同一个入口：常驻但低调，省得靠猜（右键仍然可用） -->
          <el-icon
            class="shrink-0 rounded p-1 text-lg transition-colors hover:bg-slate-200 dark:hover:bg-slate-700 hover:text-slate-700 dark:hover:text-slate-200"
            :class="selected === row.path ? 'text-slate-500 dark:text-slate-400' : 'text-slate-300 dark:text-slate-600'"
            :title="t('tree.more')"
            @click.stop="openMenu(row.path, row.dir, $event)"
          >
            <MoreFilled />
          </el-icon>
        </div>

        <div
          v-if="editing && editing.parent === row.path && row.dir"
          class="flex items-center gap-1 py-[3px] pr-1.5"
          :style="{ paddingLeft: `${(row.depth + 1) * 12 + 22}px` }"
        >
          <el-icon class="shrink-0 text-slate-400 dark:text-slate-500">
            <Folder v-if="editing.dir" />
            <Document v-else />
          </el-icon>
          <input
            ref="createInput"
            v-model="editing.name"
            class="mono min-w-0 flex-1 rounded border border-sky-400 px-1 text-sm outline-none"
            :placeholder="t('tree.namePrompt')"
            @keydown.enter.stop="commitCreate"
            @keydown.esc.stop="cancelEdit"
            @blur="cancelEdit"
          />
        </div>
      </template>

      <div
        v-if="editing && editing.parent === project"
        class="flex items-center gap-1 px-1.5 py-[3px]"
      >
        <el-icon class="shrink-0 text-slate-400 dark:text-slate-500">
          <Folder v-if="editing.dir" />
          <Document v-else />
        </el-icon>
        <input
          ref="createInput"
          v-model="editing.name"
          class="mono min-w-0 flex-1 rounded border border-sky-400 px-1 text-sm outline-none"
          :placeholder="t('tree.namePrompt')"
          @keydown.enter.stop="commitCreate"
          @keydown.esc.stop="cancelEdit"
          @blur="cancelEdit"
        />
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
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  CaretBottom,
  CaretRight,
  Document,
  Expand,
  Fold,
  Folder,
  FolderOpened,
  MoreFilled,
  Rank,
  Refresh,
  Switch,
} from '@element-plus/icons-vue'
import { useUpload } from './useUpload'
import { useDownload } from './useDownload'
import { useWorkspaceStore } from '@/stores/workspace'
import { isArchive } from '@/utils/format'

const props = defineProps<{ project: string }>()
const emit = defineEmits<{ resizeStart: [event: PointerEvent] }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const { upload, cancel: cancelUpload, busy: uploading, percent: uploadPercent } = useUpload()
const { download } = useDownload()

const state = computed(() => workspace.state(props.project))

const rows = computed(() => {
  const out: { path: string; name: string; dir: boolean; depth: number }[] = []
  const walk = (path: string, depth: number) => {
    for (const entry of state.value.dirs[path] ?? []) {
      const child = `${path}/${entry.name}`
      out.push({ path: child, name: entry.name, dir: entry.dir, depth })
      if (entry.dir && state.value.expanded[child]) walk(child, depth + 1)
    }
  }
  walk(props.project, 0)
  return out
})

const selected = ref(props.project)
const renaming = ref('')
const renameValue = ref('')
const editing = ref<{ parent: string; name: string; dir: boolean } | null>(null)
const renameInput = ref<HTMLInputElement[] | null>(null)
const createInput = ref<HTMLInputElement[] | null>(null)

/**
 * 模板里的 ref 在 v-for 内部会变成数组、在外部是单个元素，
 * 这里统一取第一个，避免每个调用点都写一遍类型判断。
 */
function firstEl(
  target: HTMLInputElement | HTMLInputElement[] | null | undefined,
): HTMLInputElement | null {
  if (!target) return null
  return Array.isArray(target) ? (target[0] ?? null) : target
}

const menu = reactive({ show: false, x: 0, y: 0, path: '', dir: false })
const menuEl = ref<HTMLElement | null>(null)

/** 上传目标目录：菜单里点「上传文件」时记下来，选完文件才真的发请求。 */
const uploadTarget = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

/** 剪贴板住在 store 里：文件树和「文件」标签页是同一批文件的两个视图，得共用一份。 */
const clipboard = computed(() => state.value.clipboard)

// ---- 展开 / 收起 ----

/** 顶栏那个手柄按下去 = 开始拖分隔线（逻辑在 ProjectTab，那里才有宽度状态）。 */
function onResizeStart(event: PointerEvent) {
  event.preventDefault()
  emit('resizeStart', event)
}

async function onClick(row: { path: string; dir: boolean }) {
  selected.value = row.path
  if (row.dir) {
    await workspace.toggleDir(props.project, row.path)
  } else {
    await workspace.openFile(props.project, row.path)
  }
}

/** 可见的目录一个都没展开 → 工具栏按钮切成「全部展开」。 */
const allCollapsed = computed(() =>
  rows.value.every((row) => !row.dir || !state.value.expanded[row.path]),
)

async function collapseAll() {
  for (const key of Object.keys(state.value.expanded)) {
    if (key !== props.project) state.value.expanded[key] = false
  }
}

async function expandAll() {
  await workspace.expandAll(props.project)
}

async function refreshAll() {
  await workspace.reloadTree(props.project)
}

function toTabs() {
  workspace.setViewMode(props.project, 'tabs')
}

// ---- 新建 / 重命名（内联输入，不弹窗） ----

function startCreate(parent: string, dir: boolean) {
  editing.value = { parent, name: '', dir }
  state.value.expanded[parent] = true
  void nextTick(() => firstEl(createInput.value)?.focus())
}

async function commitCreate() {
  const current = editing.value
  if (!current || !current.name.trim()) {
    cancelEdit()
    return
  }
  try {
    await workspace.createEntryAt(props.project, current.parent, current.name.trim(), current.dir)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    editing.value = null
  }
}

function startRename(path: string) {
  renaming.value = path
  renameValue.value = path.split('/').pop() ?? ''
  void nextTick(() => {
    const el = firstEl(renameInput.value)
    el?.focus()
    el?.select()
  })
}

async function commitRename(path: string) {
  const next = renameValue.value.trim()
  renaming.value = ''
  if (!next || next === path.split('/').pop()) return
  const to = `${path.slice(0, path.lastIndexOf('/'))}/${next}`
  try {
    await workspace.renameEntryAt(props.project, path, to)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

function cancelEdit() {
  editing.value = null
  renaming.value = ''
}

// ---- 右键菜单 ----

/** 菜单离窗口边缘至少留这么多，免得被切掉 */
const MENU_MARGIN = 8

function openMenu(path: string, dir: boolean, event: MouseEvent) {
  selected.value = path
  menu.show = true
  menu.x = event.clientX
  menu.y = event.clientY
  menu.path = path
  menu.dir = dir
  // 位置先按指针摆好，渲染出来量到真实尺寸后再夹回窗口内（一帧之内，看不出来）
  void nextTick(clampMenu)
  window.setTimeout(() => window.addEventListener('click', hideMenu, { once: true }), 0)
}

/** 贴近右 / 下边缘点击时菜单会有一部分跑到窗口外，这里把它收回来。 */
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

/** 菜单所在的行对应的目录：目录就是它自己，文件取父目录，项目根就是项目根。 */
const menuDir = computed(() => {
  const path = menu.path
  if (path === props.project) return props.project
  return menu.dir ? path : path.slice(0, path.lastIndexOf('/'))
})

/** 剪切中的那一项（在树里淡一半，粘完就没了） */
const cutPath = computed(() => (clipboard.value?.cut ? clipboard.value.path : ''))

const canPaste = computed(() => workspace.canPasteTo(props.project, menuDir.value))

/** 粘贴到菜单指向的目录；目标重名时复制换名字、剪切直接拒绝。 */
async function paste() {
  const clip = clipboard.value
  if (!clip) return
  const destDir = menuDir.value
  const name = clip.path.split('/').pop() ?? ''
  hideMenu()
  try {
    const outcome = await workspace.pasteInto(props.project, destDir)
    if (outcome === 'exists') {
      ElMessage.warning(t('tree.pasteExists', { name }))
    } else if (outcome === 'same') {
      ElMessage.info(t('tree.pasteSame', { name }))
    } else if (outcome === 'unreadable') {
      ElMessage.error(t('tree.pasteListFailed', { name: destDir.split('/').pop() ?? destDir }))
    }
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

// ---- 上传 ----

function pickFiles(dir: string) {
  uploadTarget.value = dir
  fileInput.value?.click()
}

async function onFiles(event: Event) {
  const files = Array.from((event.target as HTMLInputElement).files ?? [])
  try {
    if (files.length) await upload(props.project, uploadTarget.value, files)
  } finally {
    if (fileInput.value) fileInput.value.value = ''
  }
}

/** 解压到压缩包所在的目录（后端调系统 tar / unzip，同名文件跳过）。 */
async function extract(path: string) {
  hideMenu()
  const name = path.split('/').pop() ?? path
  try {
    const res = await workspace.extractAt(props.project, path)
    if (res.skipped > 0) {
      ElMessage.warning(
        t('tree.extractSkipped', { name, files: res.files, skipped: res.skipped }),
      )
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

/** 删除前确认；删掉的如果是剪贴板里那一项，顺手把剪贴板清掉。 */
async function remove(path: string, dir: boolean) {
  const name = path.split('/').pop() ?? path
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
  const isRoot = path === props.project
  const dir = menuDir.value
  const name = path.split('/').pop() ?? path
  const items: MenuItem[] = []

  // 「新建文件 / 新建目录」只给目录：文件的菜单里再放这两个，点了其实是建在它的
  // 父目录里，和用户看着的那一行对不上。
  if (menu.dir) {
    items.push(
      { label: t('tree.newFile'), run: () => startCreate(dir, false) },
      { label: t('tree.newDir'), run: () => startCreate(dir, true) },
      { label: t('tree.upload'), run: () => pickFiles(dir) },
    )
  }
  if (clipboard.value) {
    items.push({ label: t('tree.paste'), disabled: !canPaste.value, run: () => void paste() })
  }
  if (isRoot) return items

  items.push({ label: t('common.rename'), run: () => startRename(path) })
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
  items.push({ label: t('common.download'), run: () => void download(path) })
  if (!menu.dir && isArchive(name)) {
    items.push({ label: t('tree.extract'), run: () => void extract(path) })
  }
  items.push({
    label: t('common.delete'),
    danger: true,
    run: () => void remove(path, menu.dir),
  })
  return items
})

onMounted(() => {
  void workspace.ensure(props.project)
})

onUnmounted(hideMenu)
</script>
