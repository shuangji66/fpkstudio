/**
 * 校验取景框（拖动 / 缩放裁剪）与图标处理的计算（src/utils/icon.ts）。
 *
 *   npm run check:icon
 *
 * 这些数字错了不会报错，只会让「框里看到的」和「写进项目的」不一样，所以：
 *  1. 初始视图必须「铺满 + 居中」；
 *  2. 任何拖动 / 缩放之后都不能在框里露出空白（覆盖不变量）；
 *  3. 缩放以指针为锚，锚点底下的图片坐标不跑；
 *  4. 取景框对应的原图矩形始终落在图片范围内；
 *  5. canvas 侧：按源矩形裁剪、圆角半径夹到一半、导出尺寸与文件名正确；
 *     SVG 底色只认十六进制写法、先垫底再画图，非方形源也铺满方图；
 *  6. 副入口图标的前缀要真的收进 app/ui/images/ 里（它直接拼进文件名）。
 *
 * canvas 用最小替身，只记录调用参数——这里验的是几何，不是渲染结果。
 */
import {
  clampView,
  cropRegion,
  entryIconPath,
  entryIconPrefixes,
  fitView,
  fitZoom,
  ICON_PREFIX_MAX,
  maxZoomOf,
  normalizeHexColor,
  normalizeIconPrefix,
  roundCorners,
  svgViewBoxSize,
  svgWithIntrinsicSize,
  viewToSource,
  withBackground,
  zoomView,
} from '../src/utils/icon.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`)
}
const near = (a, b, eps = 1e-6) => Math.abs(a - b) < eps

const STAGE = 256
const wide = { w: 800, h: 400 }
const tall = { w: 400, h: 1000 }
const square = { w: 64, h: 64 }

// ---- 初始视图 ----
eqView('宽图铺满（缩放 = 框 / 短边）', fitView(wide, STAGE), {
  zoom: STAGE / 400,
  x: (STAGE - 800 * (STAGE / 400)) / 2,
  y: 0,
})
eqView('长图铺满', fitView(tall, STAGE), { zoom: STAGE / 400, x: 0, y: (STAGE - 1000 * (STAGE / 400)) / 2 })
eqView('小图放大到铺满', fitView(square, STAGE), { zoom: STAGE / 64, x: 0, y: 0 })

function eqView(name, view, want) {
  const ok = ['zoom', 'x', 'y'].every((key) => near(view[key], want[key]))
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(view)}\n     want ${JSON.stringify(want)}`}`)
}

// ---- 覆盖不变量：拖动 / 缩放到任何程度都不能露空白 ----
const covers = (view, size) =>
  view.x <= 1e-6 &&
  view.y <= 1e-6 &&
  view.x + size.w * view.zoom >= STAGE - 1e-6 &&
  view.y + size.h * view.zoom >= STAGE - 1e-6

for (const size of [wide, tall, square]) {
  const label = `${size.w}x${size.h}`
  check(`覆盖：初始 ${label}`, covers(fitView(size, STAGE), size), true)
  check(`覆盖：拖到右下被夹住 ${label}`, covers(clampView({ ...fitView(size, STAGE), x: 9999, y: 9999 }, size, STAGE), size), true)
  check(`覆盖：拖到左上被夹住 ${label}`, covers(clampView({ ...fitView(size, STAGE), x: -9999, y: -9999 }, size, STAGE), size), true)
  const zoomed = zoomView(fitView(size, STAGE), size, STAGE, 999)
  check(`缩放到上限 ${label}`, near(zoomed.zoom, maxZoomOf(size, STAGE)), true)
  check(`覆盖：放到最大仍铺满 ${label}`, covers(zoomed, size), true)
  const shrunk = zoomView(zoomed, size, STAGE, 1e-6)
  check(`缩小到下限 = 铺满 ${label}`, near(shrunk.zoom, fitZoom(size, STAGE)), true)
  check(`覆盖：缩到最小仍铺满 ${label}`, covers(shrunk, size), true)
}

// ---- 以锚点缩放 ----
{
  const base = fitView(wide, STAGE)
  const anchor = { x: 40, y: 200 }
  const imageAt = (view) => ({ x: (anchor.x - view.x) / view.zoom, y: (anchor.y - view.y) / view.zoom })
  const before = imageAt(base)
  const after = imageAt(zoomView(base, wide, STAGE, base.zoom * 2, anchor.x, anchor.y))
  check('锚点缩放：锚点下的图片坐标不动（x）', near(before.x, after.x), true)
  check('锚点缩放：锚点下的图片坐标不动（y）', near(before.y, after.y), true)
}

// ---- 取景框 → 原图矩形 ----
{
  let inside = true
  for (const zoom of [0.32, 0.5, 0.8, 1, 2, 2.56, 4]) {
    for (const [ax, ay] of [[0, 0], [STAGE, 0], [0, STAGE], [STAGE, STAGE], [17, 233]]) {
      const view = clampView(zoomView(fitView(wide, STAGE), wide, STAGE, zoom, ax, ay), wide, STAGE)
      const src = viewToSource(view, STAGE)
      const ok = near(src.side, STAGE / view.zoom) && src.sx >= -1e-6 && src.sy >= -1e-6 && src.sx + src.side <= wide.w + 1e-6 && src.sy + src.side <= wide.h + 1e-6
      if (!ok) {
        inside = false
        console.log('     越界:', JSON.stringify({ zoom, view, src }))
      }
    }
  }
  check('源矩形：各种缩放下都在图片范围内', inside, true)
  const centered = viewToSource(fitView(wide, STAGE), STAGE)
  check('源矩形：居中时左右留白相等', Math.round(centered.sx), Math.round(wide.w - centered.sx - centered.side))
}

// ---- canvas 侧 ----
class FakeCtx {
  constructor(canvas) {
    this.canvas = canvas
    this.calls = []
    this.ops = []
  }
  set imageSmoothingEnabled(_) {}
  set imageSmoothingQuality(_) {}
  set globalCompositeOperation(value) {
    this.ops.push(value)
  }
  set fillStyle(value) {
    this.ops.push(['fillStyle', value])
  }
  drawImage(...args) {
    this.calls.push(args.map((a) => (a === this.canvas ? 'self' : a)))
  }
  fillRect(...args) {
    this.ops.push(['fillRect', ...args])
  }
  beginPath() {}
  moveTo(...args) {
    this.ops.push(['moveTo', ...args])
  }
  lineTo() {}
  arcTo() {}
  closePath() {}
  fill() {}
}
class FakeCanvas {
  constructor() {
    this.width = 0
    this.height = 0
    this.ctx = new FakeCtx(this)
  }
  getContext() {
    return this.ctx
  }
  toDataURL() {
    return `data:image/png;base64,${this.width}x${this.height}`
  }
  toBlob(cb) {
    cb(new Blob([`png ${this.width}x${this.height}`], { type: 'image/png' }))
  }
}
class FakeImage {
  constructor(w, h) {
    this.naturalWidth = w
    this.naturalHeight = h
  }
}
globalThis.HTMLCanvasElement = FakeCanvas
globalThis.HTMLImageElement = FakeImage
const created = []
globalThis.document = {
  createElement: () => {
    const canvas = new FakeCanvas()
    created.push(canvas)
    return canvas
  },
}

const image = new FakeImage(800, 400)
const cropped = cropRegion(image, 320, 64, 512, 256)
check('cropRegion：输出边长', [cropped.width, cropped.height], [256, 256])
check('cropRegion：按源矩形绘制', cropped.ctx.calls[0].slice(1), [320, 64, 512, 512, 0, 0, 256, 256])

// ---- SVG 底色：先垫底再画图，且只认十六进制写法 ----
check('底色：三位缩写补全', normalizeHexColor('#abc'), '#AABBCC')
check('底色：不带 # 也认', normalizeHexColor('ff0000'), '#FF0000')
check('底色：六位大小写归一', normalizeHexColor('#00ff7F'), '#00FF7F')
check(
  '底色：非法写法不认（空串 = 不填，由调用方处理）',
  [normalizeHexColor('#12'), normalizeHexColor('red'), normalizeHexColor('#1234567'), normalizeHexColor('')],
  [null, null, null, null],
)
const withBg = withBackground(cropped, '#112233')
check('底色：输出与工作尺寸一致', [withBg.width, withBg.height], [256, 256])
check('底色：先整张填色', withBg.ctx.ops.find((op) => Array.isArray(op) && op[0] === 'fillRect'), ['fillRect', 0, 0, 256, 256])
check('底色：填的是规整后的颜色', withBg.ctx.ops.find((op) => Array.isArray(op) && op[0] === 'fillStyle'), ['fillStyle', '#112233'])
check('底色：再把原图按方图铺满', withBg.ctx.calls.map((args) => args.slice(1)), [[0, 0, 256, 256]])
const stretched = withBackground(new FakeImage(800, 400), '#112233')
check('底色：非方形源也铺满方图', stretched.ctx.calls[0].slice(1), [0, 0, 800, 800])

created.length = 0
const rounded = roundCorners(cropped, 500)
const mask = created[created.length - 1]
check('圆角：半径夹到边长一半', mask.ctx.ops.find((op) => Array.isArray(op) && op[0] === 'moveTo'), ['moveTo', 128, 0])
check('圆角：蒙版与画布同尺寸', [mask.width, mask.height], [256, 256])
check('圆角：输出用 destination-out 擦角', rounded.ctx.ops.includes('destination-out'), true)
check('圆角：半径 0 时不额外拷贝', roundCorners(cropped, 0) === cropped, true)

const { rasterToPngFile } = await import('../src/utils/icon.ts')
const file = await rasterToPngFile(rounded, 64, 'ICON.PNG')
check('导出：文件名', file.name, 'ICON.PNG')
check('导出：类型', file.type, 'image/png')
check('导出：按目标尺寸渲染', await file.text(), 'png 64x64')

// ---- 副入口图标：前缀要真的只影响文件名，带不出 app/ui/images/ ----
check('副入口路径：大图', entryIconPath('niubi', 256), 'app/ui/images/niubi_256.png')
check('副入口路径：小图', entryIconPath('niubi', 64), 'app/ui/images/niubi_64.png')
check('副入口路径：前缀为空时不生成', entryIconPath('  ', 256), '')
check('副入口路径：全是非法字符时不生成', entryIconPath('中文/名', 256), '')

{
  // 前缀就是文件名的一部分，任何形式的目录跳转都不该留在结果里。
  // 判据是「结果严格等于 app/ui/images/ 加一段文件名」——带 `/` 的跳转到这里就散了。
  const escaped = entryIconPath('../../etc/passwd', 256)
  const dir = escaped.slice(0, escaped.lastIndexOf('/'))
  const rel = escaped.slice(dir.length + 1)
  check('副入口路径：目录仍是 app/ui/images', dir, 'app/ui/images')
  check('副入口路径：文件名不是 . 或 ..', rel !== '.' && rel !== '..', true)
  check('副入口路径：空格被剔除', normalizeIconPrefix(' my app '), 'myapp')
  check('副入口路径：超长前缀被截断', normalizeIconPrefix('a'.repeat(100)).length, ICON_PREFIX_MAX)
}

// ---- 副入口的发现：导入的多入口项目里已有的图标要能被认出来 ----
check(
  '副入口发现：只认 64 / 256，其它分辨率与无关文件不算',
  entryIconPrefixes(['icon_256.png', 'niubi_256.png', 'niubi_64.png', 'foo_128.png', 'bar_512.png', 'readme.txt']),
  ['niubi'],
)
check('副入口发现：只有一个尺寸也算（另一个留空、写入时补全）', entryIconPrefixes(['second_64.png']), ['second'])
check('副入口发现：去重 + 排序', entryIconPrefixes(['b_64.png', 'a_256.png', 'a_64.png', 'b_256.png']), ['a', 'b'])
check('副入口发现：排掉默认入口 icon（含大小写）', entryIconPrefixes(['icon_64.png', 'icon_256.png', 'ICON_256.PNG']), [])
check('副入口发现：不是入口图标的文件名不算', entryIconPrefixes(['niubi.png', '_64.png', 'niubi_256.jpg', 'niubi_256.png.bak']), [])
check('副入口发现：前缀里带下划线也认得', entryIconPrefixes(['my_app_256.png', 'my_app_64.png']), ['my_app'])

// ---- SVG 固有尺寸：只写 viewBox 的小图标要能铺满取景框 ----
check('SVG 尺寸：从 viewBox 取宽高', svgViewBoxSize('<svg viewBox="0 0 16 16"></svg>'), { w: 16, h: 16 })
check('SVG 尺寸：逗号分隔的 viewBox', svgViewBoxSize("<svg viewBox='0, 0, 24, 12'></svg>"), { w: 24, h: 12 })
check('SVG 尺寸：没有 viewBox 返回 null', svgViewBoxSize('<svg width="16" height="16"></svg>'), null)
check('SVG 尺寸：viewBox 非法返回 null', svgViewBoxSize('<svg viewBox="0 0 0 16"></svg>'), null)

{
  const fixed = svgWithIntrinsicSize('<svg viewBox="0 0 24 12"><path d="M0 0"/></svg>')
  check('SVG 尺寸：补上 viewBox 的宽高', [fixed?.includes('width="24"'), fixed?.includes('height="12"')], [true, true])
  const overwritten = svgWithIntrinsicSize('<svg width="100%" height="100%" viewBox="0 0 24 24"></svg>')
  check(
    'SVG 尺寸：覆盖百分比宽高',
    [overwritten?.includes('width="24"'), overwritten?.includes('height="24"'), overwritten?.includes('100%')],
    [true, true, false],
  )
  check('SVG 尺寸：没有 viewBox 时不改', svgWithIntrinsicSize('<svg width="16" height="16"></svg>'), null)
  check('SVG 尺寸：本来就一致时不重复写', svgWithIntrinsicSize('<svg width="16" height="16" viewBox="0 0 16 16"></svg>'), null)
}

console.log(failed ? `\n${failed} 项不通过` : '\n取景框与图标处理检查全部通过')
process.exit(failed ? 1 : 0)
