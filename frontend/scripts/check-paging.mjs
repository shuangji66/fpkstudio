/**
 * 首页项目的排版算术 / 文案映射（src/utils/paging.ts）与置顶排序（src/utils/pins.ts）。
 *
 *   npm run check:paging
 *
 * 这段逻辑决定「一页放几个项目卡片」：算大了最后一行会被容器切掉一半，算小了明明
 * 有空间却少放一行——两种都只在特定窗口尺寸下出现，光看代码看不出来，所以拿一组
 * 具体的宽高钉住。间距的加减最容易差一个（N 个格子只有 N-1 个间距），这里专门验它。
 */
import {
  CARD_GAP,
  CARD_MIN_HEIGHT,
  CARD_MIN_WIDTH,
  clampPage,
  fitGrid,
  rankKeyFor,
} from '../src/utils/paging.ts'
import { addPin, removePin, renamePin, sortPinned, togglePin } from '../src/utils/pins.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`)
}

const W = CARD_MIN_WIDTH
const H = CARD_MIN_HEIGHT
const G = CARD_GAP

// ---- 列数 ----
check('一个卡片宽的位置只能放一列', fitGrid(W, H).cols, 1)
check('两个卡片加一个间距刚好放两列', fitGrid(W * 2 + G, H).cols, 2)
check('差 1px 就退回一列', fitGrid(W * 2 + G - 1, H).cols, 1)
check('列宽度按可用宽度等分', fitGrid(W * 2 + G, H).colWidth, W)
check('多出来的宽度平摊到各列', fitGrid(W * 3 + G * 2 + 30, H).colWidth, (W * 3 + G * 2 + 30 - G * 2) / 3)

// ---- 行数 ----
check('高度只够一行就是一行', fitGrid(W, H).rows, 1)
check('两行加一个行间距刚好放两行', fitGrid(W, H * 2 + G).rows, 2)
check('差 1px 就退回一行', fitGrid(W, H * 2 + G - 1).rows, 1)

// ---- 一页几个 ----
check('一页 = 列数 × 行数', fitGrid(W * 4 + G * 3, H * 5 + G * 4).perPage, 20)
check('一页至少一个（容器还没量出来时也要有东西可渲染）', fitGrid(0, 0).perPage, 1)
check('负数尺寸也给 1（折叠 / 隐藏状态下量到过）', fitGrid(-100, -100).perPage, 1)
check('很窄很高时只放一列多行', fitGrid(W, H * 6 + G * 5).perPage, 6)

// ---- 自定义约束：传进来的最小尺寸也要按同一套算法算 ----
check('自定义最小宽高', fitGrid(600, 400, { minWidth: 200, minHeight: 100, gap: 10 }), {
  cols: 2,
  rows: 3,
  perPage: 6,
  colWidth: 295,
})
check('间距为 0 也算得对', fitGrid(W * 3, H * 2, { gap: 0 }).perPage, 6)

// ---- 页码钳位 ----
check('总数 0 → 第 1 页', clampPage(5, 0, 10), 1)
check('页码落在范围内不动', clampPage(2, 50, 10), 2)
check('页码超了就退回最后一页', clampPage(9, 25, 10), 3)
check('页码小于 1 时回到第 1 页', clampPage(0, 25, 10), 1)
check('每页 0 个时按「每页 1 个」算（容器还没量出来）', clampPage(3, 25, 0), 3)
check('最后一页正好整除', clampPage(3, 30, 10), 3)

// ---- 数量称号：边界上的数量归到下面那一档（用户给的区间是叠着的） ----
check('0 个是无名之辈', rankKeyFor(0), 'home.ranks.tier0')
check('1 个仍是无名之辈', rankKeyFor(1), 'home.ranks.tier0')
check('2 个才小试牛刀', rankKeyFor(2), 'home.ranks.tier1')
check('5 个仍在第一档', rankKeyFor(5), 'home.ranks.tier1')
check('6 个进第二档', rankKeyFor(6), 'home.ranks.tier2')
check('10 个仍在第二档', rankKeyFor(10), 'home.ranks.tier2')
check('11 个进第三档', rankKeyFor(11), 'home.ranks.tier3')
check('20 个仍在第三档', rankKeyFor(20), 'home.ranks.tier3')
check('21 个进第四档', rankKeyFor(21), 'home.ranks.tier4')
check('50 个仍在第四档', rankKeyFor(50), 'home.ranks.tier4')
check('51 个才是打包之神', rankKeyFor(51), 'home.ranks.tier5')
check('负数也走第一档（这一行本来就不渲染）', rankKeyFor(-3), 'home.ranks.tier0')

// ---- 置顶：增删、顺序、稳定性 ----
const P = (...names) => names.map((name) => ({ name }))

check('置顶表为空时原样返回（顺序不动）', sortPinned(P('a', 'b', 'c'), []), P('a', 'b', 'c'))
check('置顶的排到最前面', sortPinned(P('a', 'b', 'c'), ['c']), P('c', 'a', 'b'))
check('多个置顶时先置顶的在前', sortPinned(P('a', 'b', 'c'), ['c', 'b']), P('c', 'b', 'a'))
check('没置顶的保持原来的相对顺序', sortPinned(P('a', 'b', 'c', 'd'), ['d', 'b']), P('d', 'b', 'a', 'c'))
check('置顶表里的陌生名字不影响排序（项目被删掉了）', sortPinned(P('a', 'b'), ['gone', 'b']), P('b', 'a'))

check('置顶一次', addPin([], 'a'), ['a'])
check('重复置顶不产生第二项', addPin(['a'], 'a'), ['a'])
check('新置顶排在已有置顶后面', addPin(['a'], 'b'), ['a', 'b'])
check('取消置顶', removePin(['a', 'b'], 'a'), ['b'])
check('取消一个没置顶的不报错', removePin(['a'], 'b'), ['a'])
check('点一下置顶', togglePin(['a'], 'b'), ['a', 'b'])
check('再点一下取消', togglePin(['a', 'b'], 'b'), ['a'])
check('取消再置顶会排到最后（最近置顶的离手最近）', addPin(removePin(['a', 'b'], 'a'), 'a'), ['b', 'a'])
check('改名时置顶跟着走', renamePin(['a', 'b'], 'a', 'c'), ['c', 'b'])
check('没置顶的项目改名不影响置顶表', renamePin(['a'], 'b', 'c'), ['a'])
check('原数组不被改动', (() => {
  const source = ['a']
  addPin(source, 'b')
  return source
})(), ['a'])

// ---- 不变量：perPage 至少 1，且列数随宽度单调不减 ----
let prev = 0
for (const w of [200, 300, 500, 600, 900, 1400, 2000]) {
  const cols = fitGrid(w, H).cols
  if (cols < prev) {
    failed++
    console.log(`FAIL 列数随宽度单调不减（宽度 ${w} 上从 ${prev} 掉到 ${cols}）`)
  }
  prev = cols
  if (fitGrid(w, H).perPage < 1) {
    failed++
    console.log(`FAIL perPage 恒 >= 1（宽度 ${w}）`)
  }
}


// ---- 置顶的顺序是稳定的：同名项目重复出现在不同的输入顺序里也要给出同一套结果 ----
{
  const once = sortPinned(P('a', 'b', 'c'), ['b'])
  const twice = sortPinned(P('a', 'b', 'c'), ['b'])
  if (JSON.stringify(once) !== JSON.stringify(twice)) {
    failed++
    console.log('FAIL 同一份输入应当给出同一套排序结果')
  }
  if (once.map((p) => p.name).join() !== 'b,a,c') {
    failed++
    console.log(`FAIL 置顶排序结果不对：${once.map((p) => p.name).join()}`)
  }
}

console.log(failed ? `\n${failed} 项不通过` : '\n分页排版检查全部通过')
process.exit(failed ? 1 : 0)
