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
        @keydown.enter="onAddEnter"
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
            class="close-x hover-reveal text-[11px] transition-opacity"
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
          <!--
            和 manifest 的表单同一套排法：一行一组字段，组里每个字段「标题一行、控件一行」，
            标题和控件都是 14px（控件显式 size="default"，全局配的是 small = 12px）。
            窄屏下 .form-stack 把每行拆成一行一个。
          -->
          <div class="form-row grid grid-cols-2 gap-x-3 gap-y-1.5 py-1.5">
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.entryId') }}</span>
              <el-input
                v-model="idDraft"
                size="default"
                class="mono"
                placeholder="myapp.Application"
                @keydown.enter="onIdEnter"
                @blur="commitId"
              />
            </div>
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.title') }}</span>
              <el-input v-model="current.value.title" size="default" />
            </div>
          </div>

          <div class="form-row grid grid-cols-2 gap-x-3 gap-y-1.5 py-1.5">
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.icon') }}</span>
              <el-input v-model="current.value.icon" size="default" placeholder="images/icon_{0}.png" />
            </div>
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.type') }}</span>
              <el-select v-model="current.value.type" size="default">
                <el-option :label="t('uiConfig.typeIframe')" value="iframe" />
                <el-option :label="t('uiConfig.typeUrl')" value="url" />
              </el-select>
            </div>
          </div>

          <!--
            协议 + 端口占一半、路径独占另一半：路径会比协议 / 端口长得多，平均分三列时
            它反而不够填。里面这一对短的也不再往下拆（窄屏下同样并排）。
          -->
          <div class="form-row grid grid-cols-2 gap-x-3 gap-y-1.5 py-1.5">
            <div class="grid grid-cols-2 gap-x-3">
              <div class="flex min-w-0 flex-col gap-1">
                <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.protocol') }}</span>
                <el-select v-model="current.value.protocol" size="default">
                  <el-option :label="t('uiConfig.autoAdaptive')" value="" />
                  <el-option label="http" value="http" />
                  <el-option label="https" value="https" />
                </el-select>
              </div>
              <div class="flex min-w-0 flex-col gap-1">
                <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.port') }}</span>
                <el-input v-model="current.value.port" size="default" placeholder="${wizard_port}" />
              </div>
            </div>
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.url') }}</span>
              <el-input v-model="current.value.url" size="default" />
            </div>
          </div>

          <div class="form-row grid grid-cols-2 gap-x-3 gap-y-1.5 py-1.5">
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.gatewayPrefix') }}</span>
              <el-input v-model="current.value.gatewayPrefix" size="default" placeholder="/app/myapp" />
            </div>
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.gatewaySocket') }}</span>
              <el-input v-model="current.value.gatewaySocket" size="default" placeholder="app.sock" />
            </div>
          </div>

          <!-- 关联文件类型：多选标签要宽度，自己占一整行（没挂 collapse-tags，会自己折行） -->
          <div class="form-row grid grid-cols-1 gap-x-3 gap-y-1.5 py-1.5">
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.fileTypes') }}</span>
              <el-select
                v-model="current.value.fileTypes"
                size="default"
                multiple
                filterable
                allow-create
                default-first-option
              >
                <el-option v-for="item in current.value.fileTypes" :key="item" :label="item" :value="item" />
              </el-select>
            </div>
          </div>

          <!-- 入口设置权限 + 两个开关一行 -->
          <div class="form-row grid grid-cols-3 gap-x-3 gap-y-1.5 py-1.5">
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.accessPerm') }}</span>
              <!-- 所有用户可见时，逐个用户的权限设置没有意义，直接禁用 -->
              <el-select
                v-model="current.value.control.accessPerm"
                size="default"
                :disabled="current.value.allUsers"
              >
                <el-option :label="t('uiConfig.permEditable')" value="editable" />
                <el-option :label="t('uiConfig.permReadonly')" value="readonly" />
                <el-option :label="t('uiConfig.permHidden')" value="hidden" />
              </el-select>
            </div>
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.allUsers') }}</span>
              <el-switch v-model="current.value.allUsers" />
            </div>
            <div class="flex min-w-0 flex-col gap-1">
              <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('uiConfig.noDisplay') }}</span>
              <el-switch v-model="current.value.noDisplay" />
            </div>
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
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, Plus } from '@element-plus/icons-vue'
import { revealActive, wheelToHorizontal } from '@/utils/scroll'
import { onEnter } from '@/utils/keyboard'
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
  /** 入口里表单不认识的键，保存时原样写回。 */
  extra: Record<string, unknown>
  /** `control` 里除了 accessPerm 之外的键，保存时原样写回。 */
  controlExtra: Record<string, unknown>
  [key: string]: unknown
}

/** 表单认识的入口字段；之外的都归到 extra 里原样保留。 */
const KNOWN_ENTRY_KEYS = [
  'title',
  'icon',
  'type',
  'protocol',
  'port',
  'url',
  'gatewayPrefix',
  'gatewaySocket',
  'fileTypes',
  'allUsers',
  'noDisplay',
  'control',
]

/** 挑出「表单不认识的键」，保存时原样写回。 */
function pickExtra(raw: Record<string, unknown>, known: string[]): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(raw)) {
    if (!known.includes(key)) out[key] = value
  }
  return out
}

interface Model {
  urls: Record<string, UiEntry>
  /** `.url` 之外的顶层键，保存时原样写回。 */
  rest: Record<string, unknown>
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
    // 不认识的键留着：这份 config 是 fnOS 读的，以后加字段不该被我们保存一次就丢掉
    // （顶层注意别丢）。已知之外的一切都进 extra / controlExtra。
    extra: pickExtra(raw, KNOWN_ENTRY_KEYS),
    controlExtra: pickExtra(control, ['accessPerm']),
  }
}

const { model, api } = useEditor<Model>(
  props,
  emit,
  (text) => {
    const parsed = parseJSON<Record<string, unknown>>(text, {})
    const raw = parsed.value ?? {}
    const rawUrls = raw['.url']
    // `.url` 不是对象时不能当成「没有入口」继续：表单会是空的，一保存就把整份文件写成
    // {".url": {}}。降级成空模型 + error，由 saveBlocked 拦住保存。
    if (
      rawUrls !== undefined &&
      (rawUrls === null || typeof rawUrls !== 'object' || Array.isArray(rawUrls))
    ) {
      return { urls: {}, rest: pickExtra(raw, ['.url']), error: '".url" is not an object' }
    }
    const urls = (rawUrls ?? {}) as Record<string, Record<string, unknown>>
    const out: Record<string, UiEntry> = {}
    for (const [id, entry] of Object.entries(urls)) out[id] = normalizeEntry(entry ?? {})
    // `.url` 之外的顶层键原样留着
    return { urls: out, rest: pickExtra(raw, ['.url']), error: parsed.error }
  },
  (value) => {
    const urls: Record<string, unknown> = {}
    for (const [id, entry] of Object.entries(value.urls)) {
      const item: Record<string, unknown> = {
        // 不认识的键原样写回（否则「改个标题保存」就把它们删了）
        ...entry.extra,
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
      // control：认识的只有 accessPerm，其余键（以后加的）一起写回；
      // 默认值 editable 不写，免得给文件凭空加键。
      const control: Record<string, unknown> = { ...entry.controlExtra }
      if (entry.control.accessPerm !== 'editable') control.accessPerm = entry.control.accessPerm
      if (Object.keys(control).length) item.control = control
      urls[id] = item
    }
    return stringifyOrdered({ ...value.rest, '.url': urls })
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

/** 回车提交入口 id；组词中的回车不算（见 utils/keyboard.ts）。 */
function onIdEnter(event: Event) {
  onEnter(event, commitId)
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

/** 回车提交新入口 id；组词中的回车不算（见 utils/keyboard.ts）。 */
function onAddEnter(event: Event) {
  onEnter(event, commitAdd)
}

/**
 * 删掉一个入口。
 *
 * 先确认：入口里可能填了一整屏内容，误点一下就没了（而且这一页没有撤销）。
 *
 * 删除入口的入口从表单里挪到了标签上的 ×，所以这里不能再无脑把选中项拨回第一个：
 * 删的是别的标签时用户的视线还留在原来那个入口上，跳走等于让他丢上下文。
 * 删的正好是当前这个，才顺势选它前面那个（第一个就往后选）。
 */
async function removeEntry(id: string) {
  try {
    await ElMessageBox.confirm(
      t('uiConfig.removeEntryConfirm', { name: id }),
      t('common.tip'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
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
