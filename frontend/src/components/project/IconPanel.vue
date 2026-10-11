<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-4">
    <div class="mx-auto max-w-3xl space-y-3">
      <div class="flex flex-wrap items-start justify-center gap-6">
        <!-- 取景框：拖动平移、滚轮/滑块缩放，框内就是将要写出去的方图 -->
        <div class="shrink-0">
          <div
            class="relative mx-auto cursor-grab touch-none overflow-hidden outline-1 outline-dashed transition-colors"
            :class="[
              image ? 'outline-amber-400' : 'outline-sky-400 hover:bg-sky-50/60 dark:hover:bg-sky-950/40',
              dragging ? 'cursor-grabbing' : '',
            ]"
            :style="stageStyle"
            :title="image ? t('iconTab.dragTip') : t('iconTab.pick')"
            @pointerdown="startDrag"
            @pointermove="onDrag"
            @pointerup="endDrag"
            @pointercancel="endDrag"
            @wheel.prevent="onWheel"
            @dragover.prevent
            @dragenter.prevent="draggingFile = true"
            @dragleave.prevent="draggingFile = false"
            @drop.prevent="onDrop"
            @click="image ? undefined : pick()"
          >
            <img
              v-if="image"
              :src="imageURL"
              class="pointer-events-none absolute max-w-none select-none"
              :style="imageStyle"
              draggable="false"
              alt=""
            />
            <span
              v-else
              class="flex h-full items-center justify-center px-4 text-center text-xs leading-relaxed text-slate-400 dark:text-slate-500"
            >
              {{ t('iconTab.pickHint') }}
            </span>

            <div
              v-if="draggingFile"
              class="absolute inset-0 flex items-center justify-center bg-white/80 dark:bg-slate-800/80 text-xs text-sky-600 dark:text-sky-400"
            >
              {{ t('iconTab.drop') }}
            </div>
          </div>

          <p class="mt-2 text-center text-[11px] text-slate-500 dark:text-slate-400">
            {{ image ? t('iconTab.cropTip') : t('iconTab.pickTip') }}
          </p>

          <div
            class="mx-auto mt-2 grid w-[16rem] grid-cols-[2.5rem_1fr] items-center gap-x-2 gap-y-1.5 text-xs text-slate-600 dark:text-slate-300"
          >
            <span>{{ t('iconTab.zoom') }}</span>
            <el-slider
              v-model="zoomSlider"
              size="small"
              :min="minZoom"
              :max="maxZoom"
              :step="0.01"
              :disabled="!image"
              :show-tooltip="false"
              @input="onZoomSlide"
            />

            <span>{{ t('iconTab.size') }}</span>
            <el-input-number
              v-model="workSize"
              size="small"
              :min="32"
              :max="4096"
              :disabled="!image"
              controls-position="right"
              class="w-28"
              @change="clampRadius"
            />

            <span>{{ t('iconTab.radius') }}</span>
            <el-slider
              v-model="radius"
              size="small"
              :min="0"
              :max="Math.floor(workSize / 2)"
              :disabled="!image"
            />

            <span />
            <div class="flex items-center gap-2">
              <el-button size="small" :icon="Picture" :disabled="!image" @click="pick">
                {{ t('iconTab.replace') }}
              </el-button>
              <el-button size="small" :disabled="!image" @click="resetView">
                {{ t('iconTab.resetView') }}
              </el-button>
            </div>
          </div>
        </div>

        <!-- 保存目标 -->
        <div class="min-w-[19rem] flex-1 space-y-2">
          <div class="label-text">{{ t('iconTab.targets') }}</div>

          <!-- 固定的四个位置 -->
          <IconTargetRow
            v-for="target in baseTargets"
            :key="target.key"
            v-model:checked="chosen[target.key]"
            :label-key="target.labelKey"
            :path="target.path"
            :size="targetSize(target)"
            :src="iconURL(target)"
          />

          <!--
            副入口图标：一个项目可以有多个入口，除默认入口（icon_256 / icon_64）之外
            的入口各自一套大 / 小图标，文件名以前缀区分（niubi → niubi_256.png）。
            导入的多入口项目里已经存在的那些，进页面时自动列出来；没写过的尺寸留空，
            勾上「生成并写入」即可补全。
          -->
          <div
            v-for="entry in entryGroups"
            :key="entry.group.id"
            class="space-y-2 rounded-md border border-dashed border-slate-300 dark:border-slate-600 p-2"
          >
            <div class="flex items-center gap-2">
              <span class="shrink-0 text-xs text-slate-600 dark:text-slate-300">{{ t('iconTab.entryPrefix') }}</span>
              <el-input
                v-model="entry.group.prefix"
                size="small"
                class="w-40"
                :maxlength="ICON_PREFIX_MAX"
                :placeholder="t('iconTab.entryPrefixPlaceholder')"
              />
              <span class="flex-1" />
              <el-button
                text
                size="small"
                :icon="Delete"
                :title="t('iconTab.removeEntry')"
                @click="removeGroup(entry.group)"
              />
            </div>

            <template v-if="entry.targets.length">
              <IconTargetRow
                v-for="target in entry.targets"
                :key="target.key"
                v-model:checked="chosen[target.key]"
                :label-key="target.labelKey"
                :path="target.path"
                :size="targetSize(target)"
                :src="iconURL(target)"
              />
            </template>
            <p v-else class="text-[11px] leading-relaxed text-slate-400 dark:text-slate-500">
              {{ t('iconTab.entryPrefixHint') }}
            </p>
          </div>

          <el-button v-if="state.ui" size="small" :icon="Plus" @click="addGroup">
            {{ t('iconTab.addEntry') }}
          </el-button>

          <div class="grid grid-cols-[auto_1fr] items-center gap-x-3 pt-1">
            <span class="text-xs text-slate-600 dark:text-slate-300">{{ t('iconTab.smallScale') }}</span>
            <el-radio-group v-model="smallScale" size="small" :disabled="!hasSmall">
              <el-radio-button :value="1">{{ t('iconTab.scaleSame') }}</el-radio-button>
              <el-radio-button :value="0.5">1/2</el-radio-button>
              <el-radio-button :value="0.25">1/4</el-radio-button>
              <el-radio-button :value="0.125">1/8</el-radio-button>
            </el-radio-group>
          </div>

          <div class="flex items-center gap-2 pt-1">
            <el-button type="primary" size="small" :icon="Upload" :loading="saving" @click="save">
              {{ saving ? t('iconTab.saving') : t('iconTab.save') }}
            </el-button>
            <span v-if="progress.total" class="mono text-[11px] text-slate-500 dark:text-slate-400">
              {{ progress.done }}/{{ progress.total }}
            </span>
          </div>

          <p class="text-[11px] leading-relaxed text-slate-400 dark:text-slate-500">{{ t('iconTab.targetHint') }}</p>
        </div>
      </div>
    </div>

    <input ref="input" type="file" accept="image/*" class="hidden" @change="onPick" />
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Delete, Picture, Plus, Upload } from '@element-plus/icons-vue'
import { rawURL } from '@/api/client'
import { listDir } from '@/api/endpoints'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  clampView,
  cropRegion,
  entryIconPath,
  entryIconPrefixes,
  ENTRY_ICON_DIR,
  fitView,
  ICON_PREFIX_MAX,
  imageFromFile,
  imageFromURL,
  maxZoomOf,
  normalizeIconPrefix,
  rasterToPngFile,
  roundCorners,
  viewToSource,
  zoomView,
  type CropView,
} from '@/utils/icon'
import IconTargetRow from './IconTargetRow.vue'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const state = computed(() => workspace.state(props.project))

/** 取景框的屏幕边长。图片按 zoom 缩放后摆在这个方框里，框内即结果。 */
const STAGE = 256

/** 进页面默认载入的底图：项目里已有的应用大图标。 */
const DEFAULT_ICON = 'ICON_256.PNG'

interface TargetDef {
  /**
   * 稳定标识。基础图标就是路径，副入口图标是「组 id + 大 / 小」——
   * 前缀边打字边变，路径跟着变，勾选状态得挂在一个不变的东西上。
   */
  key: string
  /** 相对项目根的路径。 */
  path: string
  /** 落地目录，相对项目根；空串表示项目根。 */
  dir: string
  labelKey: string
  /** 基准尺寸；小图再乘上小图缩放比例。 */
  size: number
  small: boolean
  /** 入口图标只存在于带桌面入口的项目里。 */
  needsUi: boolean
}

/**
 * 四个固定的图标位置，取值和 fnpack 打包时用的一致：
 * 应用图标在包根，入口图标在 app/ui/images/，各是大图 256 + 小图 64。
 */
const TARGET_DEFS: TargetDef[] = [
  {
    key: 'ICON_256.PNG',
    path: 'ICON_256.PNG',
    dir: '',
    labelKey: 'iconTab.appLarge',
    size: 256,
    small: false,
    needsUi: false,
  },
  {
    key: 'ICON.PNG',
    path: 'ICON.PNG',
    dir: '',
    labelKey: 'iconTab.appSmall',
    size: 64,
    small: true,
    needsUi: false,
  },
  {
    key: 'app/ui/images/icon_256.png',
    path: 'app/ui/images/icon_256.png',
    dir: ENTRY_ICON_DIR,
    labelKey: 'iconTab.entryLarge',
    size: 256,
    small: false,
    needsUi: true,
  },
  {
    key: 'app/ui/images/icon_64.png',
    path: 'app/ui/images/icon_64.png',
    dir: ENTRY_ICON_DIR,
    labelKey: 'iconTab.entrySmall',
    size: 64,
    small: true,
    needsUi: true,
  },
]

/** 一个副入口图标组：前缀决定文件名（niubi → niubi_256.png / niubi_64.png）。 */
interface EntryGroup {
  id: number
  prefix: string
}

const groups = ref<EntryGroup[]>([])
let nextGroupId = 1

/** 一组副入口里的一个目标；前缀还没填时返回 null（界面上只留前缀输入框）。 */
function groupTarget(group: EntryGroup, small: boolean): TargetDef | null {
  const side = small ? 64 : 256
  const path = entryIconPath(group.prefix, side)
  if (!path) return null
  return {
    key: `${group.id}:${small ? 'small' : 'large'}`,
    path,
    dir: ENTRY_ICON_DIR,
    labelKey: small ? 'iconTab.entrySmall' : 'iconTab.entryLarge',
    size: side,
    small,
    needsUi: true,
  }
}

/** 勾选状态按 TargetDef.key 记：目标列表跟着项目形态 / 前缀重算时不会丢。 */
const chosen = reactive<Record<string, boolean>>(
  Object.fromEntries(TARGET_DEFS.map((def) => [def.key, true])),
)

/**
 * 项目里已经存在的图标文件（相对项目根的路径）。
 *
 * 两个用处：给每一行配缩略图（没写过的位置留空框），以及认出导入的多入口项目里
 * 已经有的副入口图标——它们原先在列表里完全看不见。
 */
const existing = ref<Set<string>>(new Set())

/** 缩略图地址；这个位置还没写过时返回空串（列表里就是一个空框）。 */
function iconURL(target: TargetDef): string {
  if (!existing.value.has(target.path)) return ''
  return rawURL('/api/fs/download', {
    path: `${props.project}/${target.path}`,
    rev: state.value.assetRev,
  })
}

/** 固定的那几个位置；不带桌面入口的项目没有后两个。 */
const baseTargets = computed(() => TARGET_DEFS.filter((def) => !def.needsUi || state.value.ui))

/** 副入口组连同它这一组的目标（大在前、小在后）。 */
const entryGroups = computed(() =>
  groups.value.map((group) => ({
    group,
    targets: [groupTarget(group, false), groupTarget(group, true)].filter(
      (target): target is TargetDef => target !== null,
    ),
  })),
)

/** 这次可能被写出去的全部目标（副入口的前缀没填时不参与）。 */
const targets = computed(() => [
  ...baseTargets.value,
  ...entryGroups.value.flatMap((entry) => entry.targets),
])

const hasSmall = computed(() => targets.value.some((def) => def.small))

/**
 * 加一组副入口图标。
 *
 * 顺带把所有图标写入勾选清掉：新增一组意味着这次要写的是另一套图标，
 * 沿用上一组的勾选很容易在点保存时把默认入口的图标一起覆盖掉。
 */
function addGroup() {
  for (const key of Object.keys(chosen)) chosen[key] = false
  const group: EntryGroup = { id: nextGroupId, prefix: '' }
  nextGroupId += 1
  chosen[`${group.id}:large`] = false
  chosen[`${group.id}:small`] = false
  groups.value.push(group)
}

function removeGroup(group: EntryGroup) {
  groups.value = groups.value.filter((item) => item.id !== group.id)
  delete chosen[`${group.id}:large`]
  delete chosen[`${group.id}:small`]
}

/**
 * 扫一遍现有的图标：项目根的应用图标（ICON_256.PNG / ICON.PNG），
 * 带桌面入口的项目再看 app/ui/images/ 里的入口图标。
 *
 * 形态（有没有桌面入口）是异步读回来的，所以入口目录只在 `state.ui` 为真时才去看：
 * 不带入口的项目里那个目录根本不存在，探它只会拿回一条
 * 「openat app/ui/images: no such file or directory」，把日志和项目状态弄脏。
 *
 * 返回入口目录里的文件名，交给 adoptEntries 认副入口。
 */
async function scanIcons(): Promise<string[]> {
  const paths = new Set<string>()
  const names: string[] = []

  const jobs: Promise<void>[] = [
    listDir(props.project)
      .then((res) => {
        for (const entry of res.entries) if (!entry.dir) paths.add(entry.name)
      })
      .catch(() => undefined),
  ]

  if (state.value.ui) {
    jobs.push(
      listDir(`${props.project}/${ENTRY_ICON_DIR}`)
        .then((res) => {
          for (const entry of res.entries) {
            if (entry.dir) continue
            names.push(entry.name)
            paths.add(`${ENTRY_ICON_DIR}/${entry.name}`)
          }
        })
        .catch(() => undefined),
    )
  }

  await Promise.all(jobs)
  existing.value = paths
  return names
}

/**
 * 把目录里已有的副入口图标补成一组一组的目标。
 *
 * 这些组默认不勾选：它们已经写好了，进来只是想改一改或者补一个尺寸，
 * 不该因为「点一下生成并写入」就把所有入口的图标一起覆盖掉。
 * 界面上已经有的前缀（用户自己加过的）不重复添加。
 */
function adoptEntries(fileNames: string[]) {
  const known = new Set(groups.value.map((group) => normalizeIconPrefix(group.prefix)))
  for (const prefix of entryIconPrefixes(fileNames)) {
    if (known.has(prefix)) continue
    const group: EntryGroup = { id: nextGroupId, prefix }
    nextGroupId += 1
    chosen[`${group.id}:large`] = false
    chosen[`${group.id}:small`] = false
    groups.value.push(group)
  }
}

// ---- 取景框状态 ----

const input = ref<HTMLInputElement | null>(null)
const draggingFile = ref(false)
const dragging = ref(false)
const saving = ref(false)
const progress = reactive({ done: 0, total: 0 })

/**
 * 原图。必须是 shallowRef：HTMLImageElement 被 reactive 包一层代理之后，
 * drawImage 拿到的就不是那张真实的图像了。
 */
const image = shallowRef<HTMLImageElement | null>(null)
const imageURL = ref('')
/** 当前图片的 object URL：换图或离开页面时释放（见 utils/icon.imageFromFile 的说明）。 */
let objectURL = ''

/** 取景框状态（位置与缩放），计算规则都在 utils/icon.ts 里。 */
const view = reactive<CropView>({ x: 0, y: 0, zoom: 1 })
const zoomSlider = ref(1)

const workSize = ref(256)
/**
 * 圆角半径，单位是「工作尺寸」的像素。
 *
 * 取景框固定 256px 显示，所以预览的圆角要按 256/workSize 换算——这和导出时
 * 「先按 workSize 切圆角、再缩到目标尺寸」是同一个比例，所见即所得。
 */
const radius = ref(0)
/**
 * 小图缩放。
 *
 * 默认「不变」（1）：小图和大图一样大，也就是 ICON.PNG 落成 256px——按需求定的。
 * 想跟文件名（icon_64.png）对上就手动选 1/4，那正好是 64。
 */
const smallScale = ref(1)

const imageStyle = computed(() => {
  const img = image.value
  if (!img) return {}
  return {
    left: `${view.x}px`,
    top: `${view.y}px`,
    width: `${img.naturalWidth * view.zoom}px`,
    height: `${img.naturalHeight * view.zoom}px`,
  }
})

const stageStyle = computed(() => ({
  width: `${STAGE}px`,
  height: `${STAGE}px`,
  borderRadius: `${radius.value * (STAGE / workSize.value)}px`,
}))

function targetSize(target: TargetDef): number {
  if (!target.small) return target.size
  return Math.max(1, Math.round(workSize.value * smallScale.value))
}

const imageSize = computed(() => ({
  w: image.value?.naturalWidth ?? 1,
  h: image.value?.naturalHeight ?? 1,
}))
/** 缩放滑块的上下限：最小是「铺满」，最大是铺满后再放大 8 倍。 */
const minZoom = computed(() => fitView(imageSize.value, STAGE).zoom)
const maxZoom = computed(() => maxZoomOf(imageSize.value, STAGE))

function applyView(next: CropView) {
  view.x = next.x
  view.y = next.y
  view.zoom = next.zoom
  zoomSlider.value = next.zoom
}

/** 复位到「铺满 + 居中」。 */
function resetView() {
  if (!image.value) return
  applyView(fitView(imageSize.value, STAGE))
}

function clampRadius() {
  const max = Math.floor(workSize.value / 2)
  if (radius.value > max) radius.value = max
}

// ---- 交互 ----

let dragStart = { x: 0, y: 0, viewX: 0, viewY: 0 }

function startDrag(event: PointerEvent) {
  if (!image.value) return
  dragging.value = true
  dragStart = { x: event.clientX, y: event.clientY, viewX: view.x, viewY: view.y }
  // 指针捕获失败不影响拖动本身（合成事件、或被浏览器拒绝时都会抛），只是为了
  // 手指/鼠标移出方框后还能继续拖。
  try {
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  } catch {
    /* 忽略 */
  }
}

function onDrag(event: PointerEvent) {
  if (!dragging.value) return
  applyView(
    clampView(
      {
        zoom: view.zoom,
        x: dragStart.viewX + (event.clientX - dragStart.x),
        y: dragStart.viewY + (event.clientY - dragStart.y),
      },
      imageSize.value,
      STAGE,
    ),
  )
}

function endDrag(event: PointerEvent) {
  if (!dragging.value) return
  dragging.value = false
  const el = event.currentTarget as HTMLElement
  try {
    if (el.hasPointerCapture?.(event.pointerId)) el.releasePointerCapture(event.pointerId)
  } catch {
    /* 忽略 */
  }
}

function onWheel(event: WheelEvent) {
  if (!image.value) return
  // 以指针位置为锚缩放，滚到哪儿放大到哪儿
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const factor = event.deltaY < 0 ? 1.1 : 1 / 1.1
  applyView(
    zoomView(
      view,
      imageSize.value,
      STAGE,
      view.zoom * factor,
      event.clientX - rect.left,
      event.clientY - rect.top,
    ),
  )
}

function onZoomSlide(value: number | number[]) {
  if (!image.value) return
  applyView(zoomView(view, imageSize.value, STAGE, Array.isArray(value) ? value[0]! : value))
}

function pick() {
  input.value?.click()
}

function onPick(event: Event) {
  const files = Array.from((event.target as HTMLInputElement).files ?? [])
  if (input.value) input.value.value = ''
  if (files[0]) void load(files[0])
}

function onDrop(event: DragEvent) {
  draggingFile.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) void load(file)
}

async function load(file: File) {
  if (!file.type.startsWith('image/')) {
    ElMessage.error(t('iconTab.notImage'))
    return
  }
  try {
    const loaded = await imageFromFile(file)
    applyImage(loaded.image, loaded.url, loaded.url)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

/**
 * 换一张底图：还掉上一张的 object URL，并复位取景框。
 *
 * `revokable` 只有本地文件（object URL）才有值——默认载入的图标用的是站内地址，
 * 不该也不会被 revoke。
 */
function applyImage(next: HTMLImageElement, url: string, revokable = '') {
  if (objectURL) URL.revokeObjectURL(objectURL)
  objectURL = revokable
  image.value = next
  imageURL.value = url
  // 换图后回到「铺满 + 居中」，避免沿用上一张的偏移
  resetView()
  clampRadius()
}

/**
 * 载入默认底图：项目里已有的应用大图标。
 *
 * 先看包根的 ICON_256.PNG，带桌面入口的项目再退回入口目录里的 icon_256.png
 * （多入口的项目常常只写过入口图标）。都没有就保持空白取景框，等用户导入。
 */
async function loadDefaultIcon() {
  const candidates = [DEFAULT_ICON]
  if (state.value.ui) candidates.push(`${ENTRY_ICON_DIR}/icon_256.png`)

  for (const path of candidates) {
    const url = rawURL('/api/fs/download', { path: `${props.project}/${path}` })
    try {
      applyImage(await imageFromURL(url), url)
      return
    } catch {
      /* 这个位置还没有图标，试下一个 */
    }
  }
}

/**
 * 进页面先把项目里已有的应用大图标载进来当底图，而不是从空白开始：
 * 多数时候用户只是想照着现有图标调一调，或者拿它给副入口做一套。
 * 同时列出已有的副入口图标——导入的多入口项目全靠这一步才看得见它们。
 *
 * 跟着 `state.ui` 走是必要的：形态还没读回来时只知道「先看包根」，读回来是带入口的
 * 项目再补一次（底图可以用入口图标兜底，副入口也在这时候列出来）。
 */
watch(
  () => state.value.ui,
  async (ui) => {
    // 已经有底图就不动它了：第二次触发时用户可能已经在拖取景框
    if (!image.value) await loadDefaultIcon()
    const names = await scanIcons()
    if (ui) adoptEntries(names)
  },
  { immediate: true },
)

onUnmounted(() => {
  if (objectURL) URL.revokeObjectURL(objectURL)
})

// ---- 输出 ----

/** 取景框当前覆盖的原图区域 → 按工作尺寸出方图 → 切圆角。 */
function renderWorking() {
  const img = image.value
  if (!img) return null
  const { sx, sy, side } = viewToSource(view, STAGE)
  return roundCorners(cropRegion(img, sx, sy, side, workSize.value), radius.value)
}

async function save() {
  if (!image.value) {
    ElMessage.warning(t('iconTab.needImage'))
    return
  }
  const working = renderWorking()
  if (!working) return
  const list = targets.value.filter((target) => chosen[target.key])
  if (!list.length) {
    ElMessage.warning(t('iconTab.needTarget'))
    return
  }

  saving.value = true
  progress.done = 0
  progress.total = list.length
  try {
    for (const target of list) {
      const name = target.path.split('/').pop()!
      const file = await rasterToPngFile(working, targetSize(target), name)
      const dir = target.dir ? `${props.project}/${target.dir}` : props.project
      await workspace.uploadTo(props.project, dir, [file], true)
      progress.done += 1
    }
    // 刚写出去的文件要立刻反映到列表上：缩略图出来，空框不再留空。
    await scanIcons()
    ElMessage.success(t('iconTab.saved', { count: progress.total }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    saving.value = false
    progress.total = 0
  }
}
</script>
