<template>
  <div class="flex h-full min-h-0 flex-col">
    <!-- 工具栏：按钮紧凑排布，窄侧边栏（最小 180px）也放得下；再窄就换行 -->
    <div class="flex shrink-0 flex-wrap items-center gap-0 border-b border-slate-200 px-1 py-0.5">
      <el-tooltip :content="t('tree.newFile')" placement="bottom">
        <el-button text size="small" class="tree-tool" :icon="DocumentAdd" @click="startCreate(false)" />
      </el-tooltip>
      <el-tooltip :content="t('tree.newDir')" placement="bottom">
        <el-button text size="small" class="tree-tool" :icon="FolderAdd" @click="startCreate(true)" />
      </el-tooltip>
      <el-tooltip :content="t('tree.upload')" placement="bottom">
        <el-button text size="small" class="tree-tool" :icon="UploadFilled" @click="pickFiles" />
      </el-tooltip>
      <el-tooltip :content="t('common.refresh')" placement="bottom">
        <el-button text size="small" class="tree-tool" :icon="Refresh" @click="refreshAll" />
      </el-tooltip>
      <el-tooltip :content="allCollapsed ? t('tree.expandAll') : t('tree.collapseAll')" placement="bottom">
        <el-button
          text
          size="small"
          class="tree-tool"
          :icon="allCollapsed ? Expand : Fold"
          @click="allCollapsed ? expandAll() : collapseAll()"
        />
      </el-tooltip>
    </div>

    <div
      class="min-h-0 flex-1 overflow-auto thin-scroll py-1 text-[13px]"
      @click.self="selected = project"
      @contextmenu.self.prevent="openMenu(project, true, $event)"
    >
      <div
        v-if="state.loading && !rows.length"
        class="px-3 py-4 text-center text-xs text-slate-400"
      >
        {{ t('common.loading') }}
      </div>

      <div v-else-if="!rows.length" class="px-3 py-4 text-center text-xs">
        <template v-if="state.error">
          <p class="text-left leading-relaxed break-all text-red-600">{{ state.error }}</p>
          <el-button text size="small" class="mt-1" :icon="Refresh" @click="refreshAll">
            {{ t('common.retry') }}
          </el-button>
        </template>
        <span v-else class="text-slate-400">{{ t('tree.emptyDir') }}</span>
      </div>

      <template v-for="row in rows" :key="row.path">
        <div
          class="group flex cursor-pointer items-center gap-1 rounded px-1.5 py-[3px] hover:bg-slate-100"
          :class="selected === row.path ? 'bg-sky-50 ring-1 ring-inset ring-sky-200' : ''"
          @click="onClick(row)"
          @contextmenu.prevent="openMenu(row.path, row.dir, $event)"
        >
          <span :style="{ width: `${row.depth * 12}px` }" class="shrink-0" />

          <el-icon v-if="row.dir" class="shrink-0 text-slate-400">
            <CaretRight v-if="!state.expanded[row.path]" />
            <CaretBottom v-else />
          </el-icon>
          <span v-else class="w-[1em] shrink-0" />

          <el-icon class="shrink-0" :class="row.dir ? 'text-amber-500' : 'text-slate-400'">
            <FolderOpened v-if="row.dir && state.expanded[row.path]" />
            <Folder v-else-if="row.dir" />
            <Document v-else />
          </el-icon>

          <template v-if="renaming === row.path">
            <input
              ref="renameInput"
              v-model="renameValue"
              class="mono min-w-0 flex-1 rounded border border-sky-400 px-1 text-[12px] outline-none"
              @click.stop
              @keydown.enter.stop="commitRename(row.path)"
              @keydown.esc.stop="cancelEdit"
              @blur="cancelEdit"
            />
          </template>
          <template v-else>
            <span class="min-w-0 flex-1 truncate" :title="row.path">{{ row.name }}</span>
            <span v-if="!row.dir && row.size >= 0" class="shrink-0 text-[10px] text-slate-400">
              {{ humanSize(row.size) }}
            </span>
          </template>

          <!-- 和右键菜单同一个入口：常驻但低调，省得靠猜（右键仍然可用） -->
          <el-icon
            class="shrink-0 rounded p-0.5 transition-colors hover:bg-slate-200 hover:text-slate-700"
            :class="selected === row.path ? 'text-slate-500' : 'text-slate-300'"
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
          <el-icon class="shrink-0 text-slate-400">
            <Folder v-if="editing.dir" />
            <Document v-else />
          </el-icon>
          <input
            ref="createInput"
            v-model="editing.name"
            class="mono min-w-0 flex-1 rounded border border-sky-400 px-1 text-[12px] outline-none"
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
        <el-icon class="shrink-0 text-slate-400">
          <Folder v-if="editing.dir" />
          <Document v-else />
        </el-icon>
        <input
          ref="createInput"
          v-model="editing.name"
          class="mono min-w-0 flex-1 rounded border border-sky-400 px-1 text-[12px] outline-none"
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
      class="fixed z-50 min-w-[9rem] rounded-md border border-slate-200 bg-white py-1 text-[13px] shadow-lg"
      :style="{ left: `${menu.x}px`, top: `${menu.y}px` }"
    >
      <button
        v-for="item in menuItems"
        :key="item.label"
        type="button"
        class="block w-full whitespace-nowrap px-3 py-1 text-left hover:bg-slate-100"
        :class="item.danger ? 'text-red-600' : ''"
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
  DocumentAdd,
  Expand,
  Folder,
  FolderAdd,
  FolderOpened,
  Fold,
  MoreFilled,
  Refresh,
  UploadFilled,
} from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { humanSize } from '@/utils/format'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const state = computed(() => workspace.state(props.project))

const rows = computed(() => {
  const out: { path: string; name: string; dir: boolean; depth: number; size: number }[] = []
  const walk = (path: string, depth: number) => {
    for (const entry of state.value.dirs[path] ?? []) {
      const child = `${path}/${entry.name}`
      out.push({ path: child, name: entry.name, dir: entry.dir, depth, size: entry.size })
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
const fileInput = ref<HTMLInputElement | null>(null)
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

const uploadTarget = computed(() => {
  if (selected.value === props.project) return props.project
  const row = rows.value.find((r) => r.path === selected.value)
  if (!row) return props.project
  return row.dir ? row.path : row.path.slice(0, row.path.lastIndexOf('/'))
})

// ---- 展开 / 收起 ----

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

// ---- 新建 / 重命名（内联输入，不弹窗） ----

function startCreate(dir: boolean) {
  const parent = uploadTarget.value
  const known = rows.value.some((r) => r.path === parent && r.dir)
  editing.value = { parent: known ? parent : props.project, name: '', dir }
  if (known) state.value.expanded[parent] = true
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

// ---- 上传 ----

function pickFiles() {
  fileInput.value?.click()
}

async function onFiles(event: Event) {
  const files = Array.from((event.target as HTMLInputElement).files ?? [])
  if (!files.length) return
  try {
    const res = await workspace.uploadTo(props.project, uploadTarget.value, files, true)
    ElMessage.success(t('tree.uploadDone', { count: res.files?.length ?? files.length }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    if (fileInput.value) fileInput.value.value = ''
  }
}

// ---- 右键菜单 ----

function openMenu(path: string, dir: boolean, event: MouseEvent) {
  selected.value = path
  menu.show = true
  menu.x = event.clientX
  menu.y = event.clientY
  menu.path = path
  menu.dir = dir
  window.setTimeout(() => window.addEventListener('click', hideMenu, { once: true }), 0)
}

function hideMenu() {
  menu.show = false
}

const menuItems = computed(() => {
  const path = menu.path
  const isRoot = path === props.project
  const parent = menu.dir ? path : path.slice(0, path.lastIndexOf('/'))
  return [
    {
      label: t('tree.newFile'),
      run: () => {
        editing.value = { parent, name: '', dir: false }
        state.value.expanded[parent] = true
        void nextTick(() => firstEl(createInput.value)?.focus())
      },
    },
    {
      label: t('tree.newDir'),
      run: () => {
        editing.value = { parent, name: '', dir: true }
        state.value.expanded[parent] = true
        void nextTick(() => firstEl(createInput.value)?.focus())
      },
    },
    ...(isRoot
      ? []
      : [
          { label: t('common.rename'), run: () => startRename(path) },
          {
            label: t('common.copy'),
            run: async () => {
              const name = path.split('/').pop() ?? ''
              const to = `${path.slice(0, path.lastIndexOf('/'))}/${name}.copy`
              try {
                await workspace.copyEntryAt(props.project, path, to)
              } catch (e) {
                ElMessage.error(e instanceof Error ? e.message : String(e))
              }
            },
          },
          { label: t('common.download'), run: () => workspace.downloadAt(path) },
          {
            label: t('common.delete'),
            danger: true,
            run: async () => {
              const name = path.split('/').pop() ?? path
              try {
                await ElMessageBox.confirm(
                  t('tree.confirmDelete', { name }),
                  t('common.tip'),
                  {
                    type: 'warning',
                    confirmButtonText: t('common.delete'),
                    cancelButtonText: t('common.cancel'),
                  },
                )
              } catch {
                return
              }
              try {
                await workspace.removeEntryAt(props.project, path, menu.dir)
              } catch (e) {
                ElMessage.error(e instanceof Error ? e.message : String(e))
              }
            },
          },
        ]),
  ]
})

onMounted(() => {
  void workspace.ensure(props.project)
})

onUnmounted(hideMenu)
</script>
