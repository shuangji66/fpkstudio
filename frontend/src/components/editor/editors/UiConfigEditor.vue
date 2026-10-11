<template>
  <div class="flex h-full min-h-0">
    <aside class="flex w-56 shrink-0 flex-col border-r border-slate-200 dark:border-slate-700">
      <header class="flex items-center gap-1 border-b border-slate-200 dark:border-slate-700 px-2 py-1.5">
        <span class="text-xs font-medium text-slate-600 dark:text-slate-300">{{ t('uiConfig.entries') }}</span>
        <span class="flex-1" />
        <el-button text size="small" :icon="Plus" @click="startAdd" />
      </header>
      <div class="min-h-0 flex-1 overflow-auto thin-scroll py-1">
        <div
          v-if="adding"
          class="px-2 py-1"
        >
          <input
            ref="addInput"
            v-model="newId"
            class="mono w-full rounded border border-sky-400 px-1 text-[12px] outline-none"
            placeholder="myapp.main"
            @keydown.enter="commitAdd"
            @keydown.esc="adding = false"
            @blur="commitAdd"
          />
        </div>
        <button
          v-for="entry in entries"
          :key="entry.id"
          type="button"
          class="block w-full truncate px-2 py-1 text-left text-[12px] hover:bg-slate-100 dark:hover:bg-slate-700"
          :class="entry.id === selected ? 'bg-sky-50 dark:bg-sky-950/40 font-medium text-sky-700 dark:text-sky-300' : 'text-slate-600 dark:text-slate-300'"
          @click="selected = entry.id"
        >
          {{ entry.id }}
        </button>
        <p v-if="!entries.length" class="px-2 py-2 text-[11px] text-slate-400 dark:text-slate-500">
          {{ t('common.empty') }}
        </p>
      </div>
    </aside>

    <div class="min-w-0 flex-1 overflow-y-auto thin-scroll px-4 py-3">
      <div v-if="model.error" class="mb-3 rounded-md bg-red-50 dark:bg-red-950/40 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {{ t('editor.readFailed') }}: {{ model.error }}
      </div>

      <template v-if="current">
        <div class="mb-3 flex items-center gap-2">
          <span class="mono text-sm font-medium text-slate-700 dark:text-slate-200">{{ current.id }}</span>
          <span class="flex-1" />
          <el-button text size="small" :icon="Delete" @click="removeEntry(current.id)">
            {{ t('uiConfig.removeEntry') }}
          </el-button>
        </div>

        <p class="mb-3 rounded-md bg-slate-50 dark:bg-slate-900 px-3 py-2 text-xs leading-relaxed text-slate-600 dark:text-slate-300">
          {{ t('uiConfig.hint') }}
        </p>

        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.entryId') }}</span>
          <el-input
            v-model="idDraft"
            size="small"
            class="mono w-64"
            placeholder="myapp.Application"
            @keydown.enter="commitId"
            @blur="commitId"
          />
        </div>

        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.title') }}</span>
          <el-input v-model="current.value.title" size="small" class="w-64" />
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.icon') }}</span>
          <el-input v-model="current.value.icon" size="small" class="w-64" placeholder="images/icon_{0}.png" />
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.type') }}</span>
          <el-select v-model="current.value.type" size="small" class="w-64">
            <el-option :label="t('uiConfig.typeIframe')" value="iframe" />
            <el-option :label="t('uiConfig.typeUrl')" value="url" />
          </el-select>
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.protocol') }}</span>
          <el-select v-model="current.value.protocol" size="small" class="w-64">
            <el-option label="（自适应）" value="" />
            <el-option label="http" value="http" />
            <el-option label="https" value="https" />
          </el-select>
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.port') }}</span>
          <el-input v-model="current.value.port" size="small" class="w-64" placeholder="${wizard_port}" />
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.url') }}</span>
          <el-input v-model="current.value.url" size="small" class="w-64" />
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.gatewayPrefix') }}</span>
          <el-input v-model="current.value.gatewayPrefix" size="small" class="w-64" placeholder="/app/myapp" />
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.gatewaySocket') }}</span>
          <el-input v-model="current.value.gatewaySocket" size="small" class="w-64" placeholder="app.sock" />
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.fileTypes') }}</span>
          <el-select
            v-model="current.value.fileTypes"
            size="small"
            class="w-64"
            multiple
            filterable
            allow-create
            default-first-option
          >
            <el-option v-for="item in current.value.fileTypes" :key="item" :label="item" :value="item" />
          </el-select>
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.accessPerm') }}</span>
          <!-- 所有用户可见时，逐个用户的权限设置没有意义，直接禁用 -->
          <el-select
            v-model="current.value.control.accessPerm"
            size="small"
            class="w-64"
            :disabled="current.value.allUsers"
          >
            <el-option :label="t('uiConfig.permEditable')" value="editable" />
            <el-option :label="t('uiConfig.permReadonly')" value="readonly" />
            <el-option :label="t('uiConfig.permHidden')" value="hidden" />
          </el-select>
        </div>
        <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
          <span />
          <div class="flex items-center gap-4">
            <el-checkbox v-model="current.value.allUsers">{{ t('uiConfig.allUsers') }}</el-checkbox>
            <el-checkbox v-model="current.value.noDisplay">{{ t('uiConfig.noDisplay') }}</el-checkbox>
          </div>
        </div>
      </template>

      <p v-else class="py-8 text-center text-xs text-slate-400 dark:text-slate-500">{{ t('common.empty') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Delete, Plus } from '@element-plus/icons-vue'
import { parseJSON, stringifyOrdered, useEditor, type EditorProps } from '../useEditor'

interface UiEntry {
  title: string
  icon: string
  type: string
  protocol: string
  port: string
  url: string
  gatewayPrefix: string
  gatewaySocket: string
  fileTypes: string[]
  allUsers: boolean
  /** 源文件里是否显式写了 allUsers（没写就别在保存时凭空加一个键）。 */
  allUsersSet: boolean
  noDisplay: boolean
  control: { accessPerm: string }
  [key: string]: unknown
}

interface Model {
  urls: Record<string, UiEntry>
  error: string
}

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()

function normalizeEntry(raw: Record<string, unknown>): UiEntry {
  const control = (raw.control ?? {}) as Record<string, unknown>
  return {
    title: String(raw.title ?? ''),
    icon: String(raw.icon ?? 'images/icon_{0}.png'),
    type: String(raw.type ?? 'iframe'),
    protocol: String(raw.protocol ?? ''),
    port: String(raw.port ?? ''),
    url: String(raw.url ?? '/'),
    gatewayPrefix: String(raw.gatewayPrefix ?? ''),
    gatewaySocket: String(raw.gatewaySocket ?? ''),
    fileTypes: Array.isArray(raw.fileTypes) ? (raw.fileTypes as string[]) : [],
    allUsers: raw.allUsers !== false,
    allUsersSet: 'allUsers' in raw,
    noDisplay: raw.noDisplay === true,
    control: { accessPerm: String(control.accessPerm ?? 'editable') },
  }
}

const { model, api } = useEditor<Model>(
  props,
  emit,
  (text) => {
    const parsed = parseJSON<Record<string, unknown>>(text, {})
    const urls = ((parsed.value ?? {})['.url'] ?? {}) as Record<string, Record<string, unknown>>
    const out: Record<string, UiEntry> = {}
    for (const [id, raw] of Object.entries(urls)) out[id] = normalizeEntry(raw ?? {})
    return { urls: out, error: parsed.error }
  },
  (value) => {
    const urls: Record<string, unknown> = {}
    for (const [id, entry] of Object.entries(value.urls)) {
      const item: Record<string, unknown> = {
        title: entry.title,
        icon: entry.icon,
        type: entry.type,
        protocol: entry.protocol,
      }
      if (entry.port) item.port = entry.port
      if (entry.url) item.url = entry.url
      if (entry.gatewayPrefix) item.gatewayPrefix = entry.gatewayPrefix
      if (entry.gatewaySocket) item.gatewaySocket = entry.gatewaySocket
      // allUsers 缺键的语义是 true，所以「取消勾选」必须显式写 false，
      // 否则文件里没有这个键 → 又被当成 true，勾选状态根本存不下来。
      // 反过来，原本没写、用户也没动过时不写，避免一保存就给文件加键。
      if (entry.allUsers === false || entry.allUsersSet) item.allUsers = entry.allUsers
      if (entry.noDisplay) item.noDisplay = true
      if (entry.fileTypes.length) item.fileTypes = [...entry.fileTypes]
      if (entry.control.accessPerm !== 'editable') item.control = { ...entry.control }
      urls[id] = item
    }
    return stringifyOrdered({ '.url': urls })
  },
)

const selected = ref(Object.keys(model.value.urls)[0] ?? '')

/**
 * 入口标识的编辑草稿。
 *
 * 标识是 `.url` 这个对象的键，不能直接 v-model 绑键——改名等于「删旧键、插新键」，
 * 得切换选中项并保持键序，所以用一个草稿值 + 失焦/回车提交。
 */
const idDraft = ref(selected.value)

watch(selected, (id) => {
  idDraft.value = id
})

function commitId() {
  const from = selected.value
  const to = idDraft.value.trim()
  const entry = model.value.urls[from]
  if (!entry) return
  if (!to || to === from) {
    idDraft.value = from
    return
  }
  if (model.value.urls[to]) {
    ElMessage.warning(t('uiConfig.entryIdExists'))
    idDraft.value = from
    return
  }
  const next: Record<string, UiEntry> = {}
  for (const [id, value] of Object.entries(model.value.urls)) next[id === from ? to : id] = value
  model.value.urls = next
  selected.value = to
}
const adding = ref(false)
const newId = ref('')
const addInput = ref<HTMLInputElement | null>(null)

const entries = computed(() =>
  Object.entries(model.value.urls).map(([id, value]) => ({ id, value })),
)

const current = computed(() => entries.value.find((e) => e.id === selected.value))

function startAdd() {
  adding.value = true
  newId.value = `${Object.keys(model.value.urls)[0]?.split('.')[0] ?? 'myapp'}.`
  void nextTick(() => addInput.value?.focus())
}

function commitAdd() {
  const id = newId.value.trim()
  adding.value = false
  if (!id || model.value.urls[id]) return
  model.value.urls[id] = normalizeEntry({ title: id.split('.')[1] ?? id })
  selected.value = id
}

function removeEntry(id: string) {
  delete model.value.urls[id]
  selected.value = Object.keys(model.value.urls)[0] ?? ''
}

defineExpose(api)
</script>
