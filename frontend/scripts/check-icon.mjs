/**
 * 校验取景框（拖动 / 缩放裁剪）与图标处理的计算（src/utils/icon.ts）。
 *
 *   npm run check:icon
 *
 * 这些数字错了不会报错，只会让「框里看到的」和「写进项目的」不一样，所以：
 *  1. 初始视图必须「整图放进框（按长边）+ 居中」——非正方形 logo 不能被裁掉两边；
 *  2. 比框小的那一轴固定在中间，比框大的轴永远盖满框（不留空白）；
 *  3. 缩放以指针为锚，锚点底下的图片坐标不跑；
 *  4. 最小缩放下取景框对应的原图矩形要包住整张图（越界的那部分是透明留白）；
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

// ---- 初始视图：整图放进框（按长边）+ 居中 ----
eqView('宽图：按长边放进来，上下留透明', fitView(wide, STAGE), {
  zoom: STAGE / 800,
  x: 0,
  y: (STAGE - 400 * (STAGE / 800)) / 2,
})
eqView('长图：按长边放进来，左右留透明', fitView(tall, STAGE), {
  zoom: STAGE / 1000,
  x: (STAGE - 400 * (STAGE / 1000)) / 2,
  y: 0,
})
eqView('方图：正好铺满整框', fitView(square, STAGE), { zoom: STAGE / 64, x: 0, y: 0 })

function eqView(name, view, want) {
  const ok = ['zoom', 'x', 'y'].every((key) => near(view[key], want[key]))
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(view)}\n     want ${JSON.stringify(want)}`}`)
}

// ---- 不变量 ----
/** 整张图都在框里（最小缩放就该这样）。 */
const insideStage = (view, size) =>
  view.x >= -1e-6 &&
  view.y >= -1e-6 &&
  view.x + size.w * view.zoom <= STAGE + 1e-6 &&
  view.y + size.h * view.zoom <= STAGE + 1e-6
/** 比框大的轴不许露空白；比框小的轴不管（那是留白）。 */
const noGap = (view, size) =>
  (size.w * view.zoom <= STAGE + 1e-6 ||
    (view.x <= 1e-6 && view.x + size.w * view.zoom >= STAGE - 1e-6)) &&
  (size.h * view.zoom <= STAGE + 1e-6 ||
    (view.y <= 1e-6 && view.y + size.h * view.zoom >= STAGE - 1e-6))
/** 比框小的轴必须居中（拖不动）。 */
const centered = (view, size) =>
  (size.w * view.zoom > STAGE + 1e-6 ||
    near(view.x, (STAGE - size.w * view.zoom) / 2)) &&
  (size.h * view.zoom > STAGE + 1e-6 || near(view.y, (STAGE - size.h * view.zoom) / 2))

for (const size of [wide, tall, square]) {
  const label = `${size.w}x${size.h}`
  const fit = fitView(size, STAGE)
  check(`完整可见：初始 ${label}`, insideStage(fit, size), true)
  check(`居中：初始 ${label}`, centered(fit, size), true)
  check(`铺满：初始 ${label}`, noGap(fit, size), true)

  // 图片有留白的那一轴没有可拖余量：往哪儿拖都还是居中。
  check(`拖动：留白轴拖不动 ${label}`, eqShape(clampView({ ...fit, x: 9999, y: 9999 }, size, STAGE), fit), true)
  check(`拖动：往左上拖也不会跑出框 ${label}`, insideStage(clampView({ ...fit, x: -9999, y: -9999 }, size, STAGE), size), true)

  const zoomed = zoomView(fit, size, STAGE, 999)
  check(`缩放到上限 ${label}`, near(zoomed.zoom, maxZoomOf(size, STAGE)), true)
  check(`铺满：放到最大仍盖满框 ${label}`, noGap(zoomed, size), true)
  check(`居中：放到最大仍守着留白轴 ${label}`, centered(zoomed, size), true)

  const shrunk = zoomView(zoomed, size, STAGE, 1e-6)
  check(`缩小到下限 = 整图放进框 ${label}`, near(shrunk.zoom, fitZoom(size, STAGE)), true)
  check(`缩小到下限后整图可见 ${label}`, insideStage(shrunk, size), true)
}

function eqShape(a, b) {
  return ['zoom', 'x', 'y'].every((key) => near(a[key], b[key]))
}

// ---- 以锚点缩放 ----
// 只有两条轴都比框大时才有「锚点」可言；最小缩放下留白轴是锁死居中的。
{
  const big = { w: 400, h: 300 }
  const base = zoomView(fitView(big, STAGE), big, STAGE, STAGE / 200)
  const anchor = { x: 40, y: 200 }
  const imageAt = (view) => ({ x: (anchor.x - view.x) / view.zoom, y: (anchor.y - view.y) / view.zoom })
  const before = imageAt(base)
  const zoomed = zoomView(base, big, STAGE, base.zoom * 2, anchor.x, anchor.y)
  const after = imageAt(zoomed)
  check('锚点缩放：锚点下的图片坐标不动（x）', near(before.x, after.x), true)
  check('锚点缩放：锚点下的图片坐标不动（y）', near(before.y, after.y), true)
}

// ---- 取景框 → 原图矩形 ----
{
  // 最小缩放：源矩形把整张图包住，越界的那部分是透明留白，不是裁切。
  let wrapped = true
  for (const [label, size] of [['宽图', wide], ['长图', tall], ['方图', square]]) {
    const src = viewToSource(fitView(size, STAGE), STAGE)
    const ok = src.sx <= 1e-6 && src.sy <= 1e-6 && src.sx + src.side >= size.w - 1e-6 && src.sy + src.side >= size.h - 1e-6
    if (!ok) {
      wrapped = false
      console.log('     没包住:', label, JSON.stringify(src))
    }
  }
  check('源矩形：最小缩放包住整张图', wrapped, true)

  // 放到「铺满」及以后：源矩形整个落在图内，这时才是真的裁切。
  let inside = true
  for (const zoom of [0.64, 0.8, 1, 2, 2.56, 4]) {
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
  check('源矩形：放大到铺满之后整个落在图片范围内', inside, true)

  const centeredSrc = viewToSource(fitView(tall, STAGE), STAGE)
  check(
    '源矩形：留白左右相等',
    Math.round(-centeredSrc.sx),
    Math.round(centeredSrc.sx + centeredSrc.side - tall.w),
  )
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

// 非正方形图按长边放进来，源矩形必然越界；方图的透明底就来自 drawImage 对越界部分的
// 裁剪（canvas 新建即全透明），不是我们去填白或者拉伸。
const containSrc = viewToSource(fitView(wide, STAGE), STAGE)
const padded = cropRegion(new FakeImage(800, 400), containSrc.sx, containSrc.sy, containSrc.side, 256)
check(
  'cropRegion：留白交给越界源矩形（上下各 64px 透明）',
  padded.ctx.calls[0].slice(1),
  [0, -200, 800, 800, 0, 0, 256, 256],
)

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
