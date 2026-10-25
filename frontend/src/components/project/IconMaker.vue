<template>
  <div class="shrink-0">
    <!--
      取景框：拖动平移、滚轮/滑块缩放，框内就是将要写出去的方图。
      根节点是 shrink-0：放进项目图标页的 flex 行时不被右侧的目标列表挤扁。
    -->
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
        class="flex h-full items-center justify-center px-4 text-center text-sm leading-relaxed text-slate-400 dark:text-slate-500"
      >
        {{ t('iconTab.pickHint') }}
      </span>

      <div
        v-if="draggingFile"
        class="absolute inset-0 flex items-center justify-center bg-white/80 dark:bg-slate-800/80 text-sm text-sky-600 dark:text-sky-400"
      >
        {{ t('iconTab.drop') }}
      </div>
    </div>

    <p class="mt-2 text-center text-sm text-slate-500 dark:text-slate-400">
      {{ image ? t('iconTab.cropTip') : t('iconTab.pickTip') }}
    </p>

    <div
      class="mx-auto mt-2 grid w-[16rem] grid-cols-[2.5rem_1fr] items-center gap-x-2 gap-y-1.5 text-sm text-slate-600 dark:text-slate-300"
    >
      <span>{{ t('iconTab.zoom') }}</span>
      <el-slider
        v-model="zoomSlider"
        size="default"
        :min="minZoom"
        :max="maxZoom"
        :step="0.01"
        :disabled="!image"
        :show-tooltip="false"
        @input="onZoomSlide"
      />

      <span>{{ t('iconTab.size') }}</span>
      <el-input-number
        v-model="size"
        size="default"
        :min="32"
        :max="4096"
        :disabled="!image"
        controls-position="right"
        class="w-28"
        @change="clampRadius"
      />

      <!--
        底色只对 SVG 有意义：PNG / JPG 本身就是矩形像素图，透明区域很少，
        「垫一层底色」既看不出来也容易把没透明的地方盖住。默认不填（= 不加底色）。
      -->
      <template v-if="svgSource">
        <span>{{ t('iconTab.background') }}</span>
        <div class="flex flex-wrap items-center gap-2">
          <el-input
            v-model="backgroundInput"
            size="default"
            class="w-28"
            maxlength="7"
            placeholder="#FFFFFF"
            :aria-label="t('iconTab.background')"
          />
          <span
            v-if="backgroundInput.trim() && !backgroundColor"
            class="text-sm text-red-600 dark:text-red-400"
          >
            {{ t('iconTab.backgroundInvalid') }}
          </span>
        </div>
      </template>

      <span>{{ t('iconTab.radius') }}</span>
      <el-slider
        v-model="radius"
        size="default"
        :min="0"
        :max="Math.floor(size / 2)"
        :disabled="!image"
      />

      <span />
      <div class="flex items-center gap-2">
        <el-button size="default" :icon="Picture" :disabled="!image" @click="pick">
          {{ t('iconTab.replace') }}
        </el-button>
        <el-button size="default" :disabled="!image" @click="resetView">
          {{ t('iconTab.resetView') }}
        </el-button>
      </div>
    </div>

    <input ref="input" type="file" accept="image/*,.svg" class="hidden" @change="onPick" />
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Picture } from '@element-plus/icons-vue'
import {
  clampView,
  cropRegion,
  fitView,
  IconError,
  imageFromFile,
  imageFromURL,
  maxZoomOf,
  normalizeHexColor,
  rasterToPngFile,
  roundCorners,
  viewToSource,
  withBackground,
  zoomView,
  type CropView,
} from '@/utils/icon'

/**
 * 图标取景框 + 输出参数（拖动平移、滚轮 / 滑块缩放、尺寸、圆角、SVG 底色）。
 *
 * 项目里的「图标」页和首页的「图标」弹窗共用这一份：两处都要「框内所见 = 导出的方图」，
 * 各写一份迟早会有一边改了圆角换算、另一边没跟上。只放取景与出图，不碰「写到哪里」。
 */
const props = defineProps<{
  /**
   * 默认底图（按顺序试，第一个能解码的就用它）。项目图标页传项目里已有的应用图标；
   * 弹窗没有项目，留空即可。
   */
  defaultSources?: string[]
}>()

/** 工作尺寸（取景框输出方图的边长）。项目图标页要拿它算小图缩放，所以做成 v-model。 */
const size = defineModel<number>('size', { default: 256 })

const emit = defineEmits<{ change: [state: { hasImage: boolean }] }>()

const { t } = useI18n()

function errText(e: unknown): string {
  if (e instanceof IconError) return t(e.key)
  return e instanceof Error ? e.message : String(e)
}

/** 取景框的屏幕边长。图片按 zoom 缩放后摆在这个方框里，框内即结果。 */
const STAGE = 256

const input = ref<HTMLInputElement | null>(null)
const draggingFile = ref(false)
const dragging = ref(false)

/**
 * 原图。必须是 shallowRef：HTMLImageElement 被 reactive 包一层代理之后，
 * drawImage 拿到的就不是那张真实的图像了。
 */
const image = shallowRef<HTMLImageElement | null>(null)
const imageURL = ref('')
/** 当前图片的 object URL：换图或离开页面时释放（见 utils/icon.imageFromFile 的说明）。 */
let objectURL = ''

/**
 * 底图有没有变化，通知外面。
 *
 * 用 watch 而不是在换图的函数里发一次：以后加「清空图片」时这里会自然回到 false，
 * 首页弹窗的「导出图片」不会停在可点状态（它只认这个布尔值）。
 */
watch(image, (img) => emit('change', { hasImage: Boolean(img) }))

/** 取景框状态（位置与缩放），计算规则都在 utils/icon.ts 里。 */
const view = reactive<CropView>({ x: 0, y: 0, zoom: 1 })
const zoomSlider = ref(1)

/**
 * 圆角半径，单位是「工作尺寸」的像素。
 *
 * 取景框固定 256px 显示，所以预览的圆角要按 256/size 换算——这和导出时
 * 「先按 size 切圆角、再缩到目标尺寸」是同一个比例，所见即所得。
 */
const radius = ref(0)

/** 当前底图是不是 SVG（只有 SVG 才显示底色输入框）。 */
const svgSource = ref(false)
/** 底色输入框里原样的文本；空串 = 不加底色。 */
const backgroundInput = ref('')
/** 规整后的颜色；非法 / 空串是空串（预览与导出都不加底色）。 */
const backgroundColor = computed(() => normalizeHexColor(backgroundInput.value) ?? '')

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
  borderRadius: `${radius.value * (STAGE / size.value)}px`,
  // SVG 的透明区域靠这一层显示出来；容器自己已经 overflow-hidden + 圆角，预览和导出一致
  backgroundColor: svgSource.value && backgroundColor.value ? backgroundColor.value : undefined,
}))

const imageSize = computed(() => ({
  w: image.value?.naturalWidth ?? 1,
  h: image.value?.naturalHeight ?? 1,
}))
/** 缩放滑块的上下限：最小是「整图放进框」（按长边），最大是再放大 8 倍。 */
const minZoom = computed(() => fitView(imageSize.value, STAGE).zoom)
const maxZoom = computed(() => maxZoomOf(imageSize.value, STAGE))

function applyView(next: CropView) {
  view.x = next.x
  view.y = next.y
  view.zoom = next.zoom
  zoomSlider.value = next.zoom
}

/** 复位到「整图放进框 + 居中」。 */
function resetView() {
  if (!image.value) return
  applyView(fitView(imageSize.value, STAGE))
}

function clampRadius() {
  const max = Math.floor(size.value / 2)
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

/** 文件名 / MIME 任一说它是 SVG 就算（部分系统的 SVG 没有 MIME）。 */
function looksSVG(name: string, type = ''): boolean {
  return type === 'image/svg+xml' || /\.svg$/i.test(name)
}

async function load(file: File) {
  const svg = looksSVG(file.name, file.type)
  if (!file.type.startsWith('image/') && !svg) {
    ElMessage.error(t('iconTab.notImage'))
    return
  }
  try {
    const loaded = await imageFromFile(file)
    applyImage(loaded.image, loaded.url, loaded.url, svg)
  } catch (e) {
    ElMessage.error(errText(e))
  }
}

/**
 * 换一张底图：还掉上一张的 object URL，并复位取景框。
 *
 * `revokable` 只有本地文件（object URL）才有值——项目里现成的图标用的是站内地址，
 * 不该也不会被 revoke。底色每次换图都清空（**默认不加底色**），免得上一次的
 * 颜色悄悄跟到新图上。
 */
function applyImage(next: HTMLImageElement, url: string, revokable = '', svg = false) {
  if (objectURL) URL.revokeObjectURL(objectURL)
  objectURL = revokable
  image.value = next
  imageURL.value = url
  svgSource.value = svg
  backgroundInput.value = ''
  // 换图后回到「整图放进框 + 居中」，避免沿用上一张的偏移
  resetView()
  clampRadius()
}

/** 载入一个站内地址的图片；调用方按顺序试，用来找项目里的默认底图。 */
async function loadFromURL(url: string): Promise<boolean> {
  try {
    const next = await imageFromURL(url)
    // 解码期间用户可能自己选了一张：默认图这时不能再盖回去
    if (image.value) return false
    applyImage(next, url, '', looksSVG(url))
    return true
  } catch {
    return false
  }
}

/**
 * 默认底图只在还没有图的时候试：第二次触发（比如项目形态读回来多了一个候选）
 * 不该把用户已经拖过的画面换掉。
 */
watch(
  () => (props.defaultSources ?? []).join('|'),
  async () => {
    if (image.value) return
    for (const url of props.defaultSources ?? []) {
      const loaded = await loadFromURL(url)
      // 载入失败就试下一个；这期间用户自己选了图也停手（别用默认图盖掉他的选择）
      if (loaded || image.value) return
    }
  },
  { immediate: true },
)

onUnmounted(() => {
  if (objectURL) URL.revokeObjectURL(objectURL)
})

// ---- 输出 ----

/** 取景框当前覆盖的原图区域 → 按工作尺寸出方图 →（SVG 底色）→ 切圆角。 */
function renderWorking() {
  const img = image.value
  if (!img) return null
  const { sx, sy, side } = viewToSource(view, STAGE)
  const cropped = cropRegion(img, sx, sy, side, size.value)
  const base = backgroundColor.value ? withBackground(cropped, backgroundColor.value) : cropped
  return roundCorners(base, radius.value)
}

/** 导出当前做成的一张 PNG（尺寸 = 工作尺寸）；没图时提示一句并返回 null。 */
async function exportPng(): Promise<File | null> {
  const working = renderWorking()
  if (!working) {
    ElMessage.warning(t('iconTab.needImage'))
    return null
  }
  try {
    return await rasterToPngFile(working, size.value, `icon-${size.value}.png`)
  } catch (e) {
    ElMessage.error(errText(e))
    return null
  }
}

defineExpose({ renderWorking, exportPng })
</script>
