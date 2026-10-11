<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-4">
    <div class="mx-auto max-w-3xl space-y-3">
      <p class="rounded-md bg-slate-50 px-3 py-2 text-xs leading-relaxed text-slate-600">
        {{ t('iconTab.hint') }}
      </p>

      <div class="flex flex-wrap items-start justify-center gap-6">
        <!-- 取景框：拖动平移、滚轮/滑块缩放，框内就是将要写出去的方图 -->
        <div class="shrink-0">
          <div
            class="relative mx-auto cursor-grab touch-none overflow-hidden outline-1 outline-dashed transition-colors"
            :class="[
              image ? 'outline-amber-400' : 'outline-sky-400 hover:bg-sky-50/60',
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
              class="flex h-full items-center justify-center px-4 text-center text-xs leading-relaxed text-slate-400"
            >
              {{ t('iconTab.pickHint') }}
            </span>

            <div
              v-if="draggingFile"
              class="absolute inset-0 flex items-center justify-center bg-white/80 text-xs text-sky-600"
            >
              {{ t('iconTab.drop') }}
            </div>
          </div>

          <p class="mt-2 text-center text-[11px] text-slate-500">
            {{ image ? t('iconTab.cropTip') : t('iconTab.pickTip') }}
          </p>

          <div
            class="mx-auto mt-2 grid w-[16rem] grid-cols-[2.5rem_1fr] items-center gap-x-2 gap-y-1.5 text-xs text-slate-600"
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

          <div
            v-for="target in targets"
            :key="target.path"
            class="flex items-center gap-2 rounded-md border border-slate-200 px-2 py-1.5"
          >
            <el-checkbox v-model="chosen[target.path]" size="small" />
            <span class="shrink-0 text-xs text-slate-700">{{ t(target.labelKey) }}</span>
            <span
              class="mono min-w-0 flex-1 truncate text-right text-[11px] text-slate-400"
              :title="target.path"
            >
              {{ target.path }}
            </span>
            <span class="mono shrink-0 text-[11px] text-slate-500">{{ targetSize(target) }}px</span>
          </div>

          <div class="grid grid-cols-[auto_1fr] items-center gap-x-3 pt-1">
            <span class="text-xs text-slate-600">{{ t('iconTab.smallScale') }}</span>
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
            <span v-if="progress.total" class="mono text-[11px] text-slate-500">
              {{ progress.done }}/{{ progress.total }}
            </span>
          </div>

          <p class="text-[11px] leading-relaxed text-slate-400">{{ t('iconTab.targetHint') }}</p>
        </div>
      </div>
    </div>

    <input ref="input" type="file" accept="image/*" class="hidden" @change="onPick" />
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Picture, Upload } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  clampView,
  cropRegion,
  fitView,
  imageFromFile,
  maxZoomOf,
  rasterToPngFile,
  roundCorners,
  viewToSource,
  zoomView,
  type CropView,
} from '@/utils/icon'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const state = computed(() => workspace.state(props.project))

/** 取景框的屏幕边长。图片按 zoom 缩放后摆在这个方框里，框内即结果。 */
const STAGE = 256

interface TargetDef {
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
    path: 'ICON_256.PNG',
    dir: '',
    labelKey: 'iconTab.appLarge',
    size: 256,
    small: false,
    needsUi: false,
  },
  { path: 'ICON.PNG', dir: '', labelKey: 'iconTab.appSmall', size: 64, small: true, needsUi: false },
  {
    path: 'app/ui/images/icon_256.png',
    dir: 'app/ui/images',
    labelKey: 'iconTab.entryLarge',
    size: 256,
    small: false,
    needsUi: true,
  },
  {
    path: 'app/ui/images/icon_64.png',
    dir: 'app/ui/images',
    labelKey: 'iconTab.entrySmall',
    size: 64,
    small: true,
    needsUi: true,
  },
]

/** 勾选状态按路径记：项目形态读回来重算目标列表时不会丢。 */
const chosen = reactive<Record<string, boolean>>(
  Object.fromEntries(TARGET_DEFS.map((def) => [def.path, true])),
)

const targets = computed(() => TARGET_DEFS.filter((def) => !def.needsUi || state.value.ui))
const hasSmall = computed(() => targets.value.some((def) => def.small))

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
 * 默认 1/4：大图 256 时小图正好 64，和文件名（icon_64.png）对得上。
 * fnpackup 默认「不变」，会让 ICON.PNG 也落成 256px。
 */
const smallScale = ref(0.25)

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
    if (objectURL) URL.revokeObjectURL(objectURL)
    objectURL = loaded.url
    image.value = loaded.image
    imageURL.value = loaded.url
    // 换图后回到「铺满 + 居中」，避免沿用上一张的偏移
    resetView()
    clampRadius()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

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
  const list = targets.value.filter((target) => chosen[target.path])
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
    ElMessage.success(t('iconTab.saved', { count: progress.total }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    saving.value = false
    progress.total = 0
  }
}
</script>
