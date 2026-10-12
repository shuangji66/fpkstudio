<template>
  <div class="flex h-full min-h-0 flex-col">
    <!--
      入口用一层标签切换，而不是左侧竖排列表：窄屏上竖排列表会把表单挤到没地方，
      而入口本来就少、名字也短，横着排一眼看全。

      「新增入口」钉在滚动区外面：它是动作不是内容，标签滚到后面它也一直在。
      入口多到放不下时标签栏横向滚动（滚轮走 wheelToHorizontal，触摸靠 .hscroll
      的 overflow-x + touch-action）。

      删入口收在标签上的 ×：表单里就不再排一行「入口标题 + 删除按钮」——那一行说的
      正是标签上已经写着的东西。× 在触屏上常显（悬停才出现的按钮在触屏上等于不存在）。
    -->
    <div class="flex shrink-0 items-stretch border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900">
      <button type="button" class="subtab subtab-idle shrink-0" @click="startAdd">
        <el-icon class="text-[13px]"><Plus /></el-icon>
        <span>{{ t('uiConfig.addEntry') }}</span>
      </button>
      <input
        v-if="adding"
        ref="addInput"
        v-model="newId"
        class="mono my-1 w-40 shrink-0 rounded border border-sky-400 px-1 text-[12px] outline-none"
        placeholder="myapp.main"
        @keydown.enter="commitAdd"
        @keydown.esc="adding = false"
        @blur="commitAdd"
      />
      <div
        ref="scroller"
        class="hscroll thin-scroll flex min-w-0 flex-1 items-stretch gap-0.5 px-1"
        @wheel="wheelToHorizontal"
      >
        <button
          v-for="entry in entries"
          :key="entry.id"
          type="button"
          class="subtab group max-w-[14rem] shrink-0"
          :class="entry.id === selected ? 'subtab-active' : 'subtab-idle'"
          :title="entry.id"
          @click="selected = entry.id"
        >
          <span class="truncate">{{ entry.id }}</span>
          <el-icon
            class="shrink-0 rounded text-[11px] text-slate-400 dark:text-slate-500 hover:bg-slate-200 dark:hover:bg-slate-700 hover:text-slate-700 dark:hover:text-slate-200"
            :class="layout.isMobile ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'"
            :title="t('uiConfig.removeEntry')"
            :aria-label="t('uiConfig.removeEntry')"
            @click.stop="removeEntry(entry.id)"
          >
            <Close />
          </el-icon>
        </button>
      </div>
    </div>

    <div
      class="min-w-0 flex-1 overflow-y-auto thin-scroll px-4 py-3"
      :class="{ 'form-stack': layout.isMobile }"
    >
      <div class="mx-auto max-w-2xl">
        <div v-if="model.error" class="mb-3 rounded-md bg-red-50 dark:bg-red-950/40 px-3 py-2 text-xs text-red-700 dark:text-red-300">
          {{ t('editor.readFailed') }}: {{ model.error }}
        </div>

        <template v-if="current">
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.entryId') }}</span>
            <el-input
              v-model="idDraft"
              size="small"
              class="mono w-64"
              placeholder="myapp.Application"
              @keydown.enter="commitId"
              @blur="commitId"
            />
          </div>

          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.title') }}</span>
            <el-input v-model="current.value.title" size="small" class="w-64" />
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.icon') }}</span>
            <el-input v-model="current.value.icon" size="small" class="w-64" placeholder="images/icon_{0}.png" />
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.type') }}</span>
            <el-select v-model="current.value.type" size="small" class="w-64">
              <el-option :label="t('uiConfig.typeIframe')" value="iframe" />
              <el-option :label="t('uiConfig.typeUrl')" value="url" />
            </el-select>
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.protocol') }}</span>
            <el-select v-model="current.value.protocol" size="small" class="w-64">
              <el-option label="（自适应）" value="" />
              <el-option label="http" value="http" />
              <el-option label="https" value="https" />
            </el-select>
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.port') }}</span>
            <el-input v-model="current.value.port" size="small" class="w-64" placeholder="${wizard_port}" />
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.url') }}</span>
            <el-input v-model="current.value.url" size="small" class="w-64" />
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.gatewayPrefix') }}</span>
            <el-input v-model="current.value.gatewayPrefix" size="small" class="w-64" placeholder="/app/myapp" />
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.gatewaySocket') }}</span>
            <el-input v-model="current.value.gatewaySocket" size="small" class="w-64" placeholder="app.sock" />
          </div>
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.fileTypes') }}</span>
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
          <div class="form-row grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.accessPerm') }}</span>
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
          <!-- 开关是小控件，窄屏下也不用分行（form-row-inline） -->
          <div class="form-row form-row-inline grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.allUsers') }}</span>
            <el-switch v-model="current.value.allUsers" />
          </div>
          <div class="form-row form-row-inline grid grid-cols-[10rem_1fr] items-center gap-x-3 py-1.5">
            <span class="form-label text-right text-xs text-slate-600 dark:text-slate-300">{{ t('uiConfig.noDisplay') }}</span>
            <el-switch v-model="current.value.noDisplay" />
          </div>
        </template>

        <p v-else class="py-8 text-center text-xs text-slate-400 dark:text-slate-500">{{ t('common.empty') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Close, Plus } from '@element-plus/icons-vue'
import { revealActive, wheelToHorizontal } from '@/utils/scroll'
import { useLayoutStore } from '@/stores/layout'
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
const layout = useLayoutStore()

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

/** 标签栏容器：切 / 新增入口时把选中项滚进可视区。 */
const scroller = ref<HTMLElement | null>(null)

watch(selected, async (id) => {
  idDraft.value = id
  await nextTick()
  revealActive(scroller.value, '.subtab-active')
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

/**
 * 删掉一个入口。
 *
 * 删除入口的入口从表单里挪到了标签上的 ×，所以这里不能再无脑把选中项拨回第一个：
 * 删的是别的标签时用户的视线还留在原来那个入口上，跳走等于让他丢上下文。
 * 删的正好是当前这个，才顺势选它前面那个（第一个就往后选）。
 */
function removeEntry(id: string) {
  const at = Object.keys(model.value.urls).indexOf(id)
  delete model.value.urls[id]
  if (selected.value !== id) return
  const rest = Object.keys(model.value.urls)
  selected.value = rest[Math.max(0, at - 1)] ?? ''
}

defineExpose({
  ...api,
  /**
   * 解析失败时拦住保存。
   *
   * 解析失败 = 表单里那份模型是空的，`getContent()` 交出去的是一份空配置。直接保存
   * 就会把用户那份（也许只是少了个括号、还能救的）文件覆盖掉——不可逆，所以宁可
   * 不让存：在源码视图里改回来再存。
   */
  saveBlocked: () =>
    model.value.error ? t('editor.formatJson', { detail: model.value.error }) : '',
})
</script>
