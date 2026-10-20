/**
 * 图标图片处理（移植自 fnpackup 的图标导入）。
 *
 * 流程：原图 → 按取景框裁成正方形 → 缩放到工作尺寸 → 切圆角 → 按目标尺寸导出 PNG。
 * 全部在浏览器里用 canvas 做，后端只负责写入，不需要额外的图像处理依赖。
 *
 * 中间结果统一用 canvas 表示（而不是 data URL 的 <img>）：canvas 之间可以直接
 * drawImage，不会像 <img src={dataURL}> 那样引入一次异步解码，可读性也更好。
 * 取景框（拖动 / 缩放）的状态留在组件里，这里只负责「按给定源矩形出一张方图」。
 */

/** 处理链里的中间结果，都是一个正方 canvas。 */
export type Raster = HTMLCanvasElement

/**
 * 图像处理失败。
 *
 * 错误里带的是 **i18n 键**而不是一句话：这个模块是纯逻辑（`check:icon` 直接跑它，
 * 不把 vue-i18n 拖进来），文案由界面层翻译——见 IconPanel 里的 errText。
 */
export class IconError extends Error {
  readonly key: string

  constructor(key: string) {
    super(key)
    this.name = 'IconError'
    this.key = key
  }
}

type Source = HTMLImageElement | Raster

/** 入口图标的落地目录：默认入口是 icon_256 / icon_64，副入口是 <前缀>_256 / <前缀>_64。 */
export const ENTRY_ICON_DIR = 'app/ui/images'

/** 入口图标前缀的长度上限（它同时是文件名的一部分）。 */
export const ICON_PREFIX_MAX = 32

/**
 * 收拾入口图标前缀。
 *
 * 前缀直接拼进文件名，所以只留文件名里安全的字符（字母数字和 `. _ -`），
 * 路径分隔符、空格、中文都被剔掉——不然用它能写到 `app/ui/images/` 外面去。
 */
export function normalizeIconPrefix(input: string): string {
  return input.trim().replace(/[^0-9A-Za-z._-]/g, '').slice(0, ICON_PREFIX_MAX)
}

/**
 * 入口图标的落地路径：前缀 + 尺寸，例如 niubi + 256 → app/ui/images/niubi_256.png。
 *
 * 前缀为空（或者全是非法字符）时返回空串，调用方据此跳过这一项。
 */
export function entryIconPath(prefix: string, side: 256 | 64): string {
  const safe = normalizeIconPrefix(prefix)
  return safe ? `${ENTRY_ICON_DIR}/${safe}_${side}.png` : ''
}

/** 入口图标文件名的形状：`<前缀>_256.png` / `<前缀>_64.png`。 */
const ENTRY_ICON_RE = /^(.+)_(256|64)\.png$/i

/** 默认入口（app/ui/config 里那个主入口）用的前缀。 */
export const DEFAULT_ENTRY_PREFIX = 'icon'

/**
 * 从 `app/ui/images/` 里的文件名取出入口图标前缀；不是入口图标就返回空串。
 *
 * 只认 64 / 256 两种分辨率：fnOS 的入口图标就用这两个尺寸，`<前缀>_128.png`
 * 这类是别的工具留下的，当成入口图标反而会多出一组空壳。
 */
export function entryIconPrefix(fileName: string): string {
  const match = ENTRY_ICON_RE.exec(fileName.trim())
  return match ? match[1]! : ''
}

/**
 * 从一个目录的文件名里认出**副入口**图标的前缀。
 *
 * 导入的多入口 fpk 里，每个入口都已经有一套 `<前缀>_256.png` / `<前缀>_64.png`，
 * 默认入口用 `icon_`，其余就是副入口。只出现其中一个尺寸（另一个还没写过）也算，
 * 界面上那一条留空、写入时可以补全。结果去重并排序，默认入口不在其中。
 */
export function entryIconPrefixes(fileNames: string[]): string[] {
  const found = new Set<string>()
  for (const name of fileNames) {
    const prefix = entryIconPrefix(name)
    if (prefix && prefix.toLowerCase() !== DEFAULT_ENTRY_PREFIX) found.add(prefix)
  }
  return [...found].sort()
}

/**
 * 从 SVG 源码里取 viewBox 的宽高。
 *
 * 小图标常常只写 `viewBox="0 0 16 16"`、不写 width / height。浏览器这时会把 `<img>`
 * 当成 300×150 的默认视口，内容按 preserveAspectRatio 居中留白；取景框按 300×150
 * 算「铺满」，方图里就只剩中间一小块（而且贴着上边）——这就是「小 SVG 铺不满画布」。
 * 返回 null 表示没有可用的 viewBox（那就按文件自己的宽高来）。
 */
export function svgViewBoxSize(source: string): { w: number; h: number } | null {
  const match = /<svg\b[^>]*\bviewBox\s*=\s*["']\s*([^"']+?)\s*["']/i.exec(source)
  if (!match) return null
  const parts = match[1]!.split(/[\s,]+/).map(Number)
  if (parts.length !== 4 || !parts.every((n) => Number.isFinite(n))) return null
  const w = parts[2]!
  const h = parts[3]!
  return w > 0 && h > 0 ? { w, h } : null
}

/** 把根 `<svg>` 上的 width / height 换成 viewBox 的尺寸（没有就补一个）。 */
function setSvgSize(tag: string, name: 'width' | 'height', value: number): string {
  const attr = new RegExp(`\\b${name}\\s*=\\s*(?:"[^"]*"|'[^']*'|[^\\s/>]+)`, 'i')
  if (attr.test(tag)) return tag.replace(attr, `${name}="${value}"`)
  return tag.replace(/<svg\b/i, `<svg ${name}="${value}"`)
}

/**
 * 给 SVG 源码补上「按 viewBox 的宽高」的固有尺寸。
 *
 * 只改根 `<svg>` 的 width / height，不动内容、不动 preserveAspectRatio：把浏览器
 * 猜的 300×150 视口换成作者写在 viewBox 里的真实比例，取景框的「铺满」才有意义。
 * 读不出 viewBox、或者根标签本来就是这个尺寸时返回 null（调用方原样使用）。
 */
export function svgWithIntrinsicSize(source: string): string | null {
  const box = svgViewBoxSize(source)
  const tag = /<svg\b[^>]*>/i.exec(source)
  if (!box || !tag) return null
  const next = setSvgSize(setSvgSize(tag[0], 'width', box.w), 'height', box.h)
  if (next === tag[0]) return null
  return source.replace(tag[0], next)
}

/** 非 SVG 原样返回；SVG 按 viewBox 规整固有尺寸（见 `svgWithIntrinsicSize`）。 */
async function normalizeSvgFile(file: File): Promise<Blob> {
  const svg = file.type === 'image/svg+xml' || /\.svg$/i.test(file.name)
  if (!svg) return file
  let source = ''
  try {
    source = await file.text()
  } catch {
    return file
  }
  const next = svgWithIntrinsicSize(source)
  return next ? new Blob([next], { type: 'image/svg+xml' }) : file
}

/**
 * 读一张本地文件为图片。
 *
 * 返回的 url 是 object URL，**不能在这里 revoke**：调用方还要拿它给 <img> 显示，
 * 撤销了那张 <img> 就永远加载不出来（解码用的那张 Image 已经拿到位图，canvas
 * 操作照旧可用，所以这个坑只表现为「预览空白、导出却正常」）。用完由调用方释放。
 *
 * SVG 先按 viewBox 规整固有尺寸（见 `normalizeSvgFile`）：只写 viewBox 的小图标
 * 否则会被当成 300×150 的视口，取景框里缩在上半部分。
 */
export async function imageFromFile(file: File): Promise<{ image: HTMLImageElement; url: string }> {
  const blob = await normalizeSvgFile(file)
  const url = URL.createObjectURL(blob)
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve({ image, url })
    image.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new IconError('iconTab.errorDecode'))
    }
    image.src = url
  })
}

/**
 * 读一张已经有了地址的图片（站内文件地址，例如项目里现成的 ICON_256.PNG）。
 *
 * 和 imageFromFile 的区别只有一处：这里的地址不是 object URL，调用方不需要
 * （也不该）去 revoke 它。解码失败直接抛错，由调用方决定是静默还是提示——
 * 「项目里还没有这个图标」是常态，不该弹错误。
 */
export function imageFromURL(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new IconError('iconTab.errorDecode'))
    image.src = url
  })
}

/**
 * 取原图上的一块正方形，输出边长 `out` 的方图（取景框的落地实现）。
 *
 * 源矩形越界时交给 `drawImage` 自己裁剪（规范要求按比例裁目标矩形），
 * 但调用方本来就会把取景框夹在图片范围内，正常不会越界。
 */
export function cropRegion(
  source: Source,
  sx: number,
  sy: number,
  side: number,
  out: number,
): Raster {
  const s = Math.max(1, side)
  return raster(out, (ctx) => {
    ctx.drawImage(source, sx, sy, s, s, 0, 0, out, out)
  })
}

/** 缩放到指定边长的正方形。 */
function resizeSquare(source: Source, side: number): Raster {
  const { w, h } = sizeOf(source)
  return raster(side, (ctx) => {
    ctx.drawImage(source, 0, 0, w, h, 0, 0, side, side)
  })
}

/**
 * 取景框状态：图片在框里的位置与缩放。
 *
 * zoom = 一个图片像素占几个屏幕像素；x/y = 图片左上角相对框左上角的偏移。
 * 这里的几个函数是纯计算，组件只负责把指针 / 滚轮事件换算成参数——取景框的
 * 「不留空白」「以指针为锚缩放」这些约束最容易写错，单独放着也方便验证。
 */
export interface CropView {
  x: number
  y: number
  zoom: number
}

export interface ImageSize {
  w: number
  h: number
}

/** 最小缩放：图片刚好铺满取景框（再小就会露出空白）。 */
export function fitZoom(size: ImageSize, stage: number): number {
  return stage / Math.min(size.w, size.h)
}

/** 最大缩放：铺满后再放大 8 倍，够看局部细节。 */
export function maxZoomOf(size: ImageSize, stage: number): number {
  return Math.max(fitZoom(size, stage) * 8, 1)
}

/** 把视图夹住：任何时候图片都要盖满取景框。 */
export function clampView(view: CropView, size: ImageSize, stage: number): CropView {
  const w = size.w * view.zoom
  const h = size.h * view.zoom
  return {
    zoom: view.zoom,
    x: Math.min(0, Math.max(stage - w, view.x)),
    y: Math.min(0, Math.max(stage - h, view.y)),
  }
}

/** 初始视图：铺满 + 居中。 */
export function fitView(size: ImageSize, stage: number): CropView {
  const zoom = fitZoom(size, stage)
  return clampView(
    { zoom, x: (stage - size.w * zoom) / 2, y: (stage - size.h * zoom) / 2 },
    size,
    stage,
  )
}

/** 以框内某一点为锚缩放：锚点底下的图片坐标保持不动，缩放才不会「跑掉」。 */
export function zoomView(
  view: CropView,
  size: ImageSize,
  stage: number,
  nextZoom: number,
  anchorX = stage / 2,
  anchorY = stage / 2,
): CropView {
  const zoom = Math.min(maxZoomOf(size, stage), Math.max(fitZoom(size, stage), nextZoom))
  const imageX = (anchorX - view.x) / view.zoom
  const imageY = (anchorY - view.y) / view.zoom
  return clampView(
    { zoom, x: anchorX - imageX * zoom, y: anchorY - imageY * zoom },
    size,
    stage,
  )
}

/** 取景框当前覆盖的原图区域（正方形）。 */
export function viewToSource(
  view: CropView,
  stage: number,
): { sx: number; sy: number; side: number } {
  return { sx: -view.x / view.zoom, sy: -view.y / view.zoom, side: stage / view.zoom }
}

/**
 * 规整十六进制颜色。
 *
 * 接受 `#RRGGBB` / `RRGGBB` / `#RGB` / `RGB`（大小写不限），统一成大写 `#RRGGBB`；
 * 空串和非法写法返回 null（调用方据此当作「不填底色」）。SVG 图标常是透明底，
 * 底色只在这一层合成，不改源文件。
 */
export function normalizeHexColor(input: string): string | null {
  const raw = input.trim().replace(/^#/, '')
  if (!/^[0-9a-fA-F]{3}$|^[0-9a-fA-F]{6}$/.test(raw)) return null
  const full = raw.length === 3 ? raw.split('').map((ch) => ch + ch).join('') : raw
  return `#${full.toUpperCase()}`
}

/**
 * 给透明底的图垫一层纯色。
 *
 * 先填满再画图，顺序不能反：反过来的话底色会盖住图片本身。SVG 的透明区域
 * 由这一层补上，圆角仍在下一步切（四角保持透明，不是涂成底色）。
 *
 * 画图时显式给 `side × side` 的目标矩形：源不一定是方图（比如直接传一张
 * `<img>` 进来），不写尺寸会按自然大小落下去、把方图铺不满。
 */
export function withBackground(source: Source, color: string): Raster {
  const side = sizeOf(source).w
  return raster(side, (ctx) => {
    ctx.fillStyle = color
    ctx.fillRect(0, 0, side, side)
    ctx.drawImage(source, 0, 0, side, side)
  })
}

/**
 * 切圆角。
 *
 * 用一个「圆角矩形以外全为不透明」的反向蒙版，以 destination-out 擦掉四角，
 * 这样切出来的是真透明，不是把四个角涂成背景色。
 */
export function roundCorners(source: Source, radius: number): Raster {
  const side = sizeOf(source).w
  const r = Math.max(0, Math.min(radius, side / 2))
  // 不切圆角就别多拷一张画布：这些处理都只往新建的画布上画，源画布不会被改，
  // 直接把源交出去是安全的（半径默认就是 0，这一步每改一次尺寸都会走到）。
  if (r <= 0 && source instanceof HTMLCanvasElement) return source
  return raster(side, (ctx) => {
    ctx.drawImage(source, 0, 0)
    if (r <= 0) return
    const mask = document.createElement('canvas')
    mask.width = side
    mask.height = side
    const mctx = mask.getContext('2d')!
    mctx.fillStyle = 'black'
    mctx.fillRect(0, 0, side, side)
    mctx.globalCompositeOperation = 'destination-out'
    mctx.beginPath()
    mctx.moveTo(r, 0)
    mctx.lineTo(side - r, 0)
    mctx.arcTo(side, 0, side, r, r)
    mctx.lineTo(side, side - r)
    mctx.arcTo(side, side, side - r, side, r)
    mctx.lineTo(r, side)
    mctx.arcTo(0, side, 0, side - r, r)
    mctx.lineTo(0, r)
    mctx.arcTo(0, 0, r, 0, r)
    mctx.closePath()
    mctx.fill()
    ctx.globalCompositeOperation = 'destination-out'
    ctx.drawImage(mask, 0, 0)
    ctx.globalCompositeOperation = 'source-over'
  })
}

/** 按目标尺寸导出 PNG 文件（透明通道保留，圆角才不会被填成黑色）。 */
export function rasterToPngFile(source: Source, side: number, name: string): Promise<File> {
  return new Promise((resolve, reject) => {
    const canvas = resizeSquare(source, side)
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new IconError('iconTab.errorExport'))
        return
      }
      resolve(new File([blob], name, { type: 'image/png', lastModified: Date.now() }))
    }, 'image/png')
  })
}

function sizeOf(source: Source): { w: number; h: number } {
  if (source instanceof HTMLCanvasElement) return { w: source.width, h: source.height }
  return { w: source.naturalWidth, h: source.naturalHeight }
}

function raster(side: number, paint: (ctx: CanvasRenderingContext2D) => void): Raster {
  const canvas = document.createElement('canvas')
  canvas.width = side
  canvas.height = side
  const ctx = canvas.getContext('2d')!
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  paint(ctx)
  return canvas
}
