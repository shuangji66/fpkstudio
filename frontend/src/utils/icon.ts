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
 * 读一张本地文件为图片。
 *
 * 返回的 url 是 object URL，**不能在这里 revoke**：调用方还要拿它给 <img> 显示，
 * 撤销了那张 <img> 就永远加载不出来（解码用的那张 Image 已经拿到位图，canvas
 * 操作照旧可用，所以这个坑只表现为「预览空白、导出却正常」）。用完由调用方释放。
 */
export function imageFromFile(file: File): Promise<{ image: HTMLImageElement; url: string }> {
  const url = URL.createObjectURL(file)
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve({ image, url })
    image.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('图片解码失败'))
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
    image.onerror = () => reject(new Error('图片解码失败'))
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
        reject(new Error('导出 PNG 失败'))
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
