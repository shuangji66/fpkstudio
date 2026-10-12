/**
 * 多选下拉的「能放下几个标签」的算术（src/utils/tags.ts）。
 *
 *   npm run check:tags
 *
 * 这段逻辑决定「选中的标签是显示出来还是被收进 +N」：算多了标签会折到第二行，
 * 算少了明明有位置却白折叠一个——两种都是肉眼一眼能看出来、但只看代码看不出来的
 * 错，所以拿一组具体宽度钉住。真实宽度由 TagSelect.vue 从 DOM 上量（这里不碰 DOM）。
 */
import { fitTagCount } from '../src/utils/tags.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`)
}

const GAP = 4
const TAGS = [90, 55, 60, 58] // nodejs_v22 / redis / erlang / minio 那种

check('没有标签时是 0', fitTagCount([], 500, GAP, 40), 0)
check('放得下就全部显示（不折叠）', fitTagCount(TAGS, 500, GAP, 40), 4)
check('正好占满也算放得下', fitTagCount(TAGS, 90 + 55 + 60 + 58 + GAP * 3, GAP, 40), 4)
check('差 1px 就折叠', fitTagCount(TAGS, 90 + 55 + 60 + 58 + GAP * 3 - 1, GAP, 40), 3)

// 折叠时「+1」也要占一格：270 = 90 + 55 + 60 + 2 个 gap(8) + gap(4) + +N(53)
check('折叠时给 +N 留位置', fitTagCount(TAGS, 270, GAP, 53), 3)
check('+N 留不够就再少显示一个', fitTagCount(TAGS, 269, GAP, 53), 2)
// +N 的宽度随 N 的位数变：同样一行，N 变成两位数就放不下第三个了
check('N 多一位要重算', fitTagCount(TAGS, 270, GAP, 60), 2)

check('只放得下一个就显示一个', fitTagCount(TAGS, 100, GAP, 40), 1)
check('一个都放不下也给 1（交给省略号）', fitTagCount(TAGS, 10, GAP, 40), 1)
check('单个标签比整行还宽也给 1', fitTagCount([400], 100, GAP, 40), 1)
check('长标签排在后面时前面照常折叠', fitTagCount([20, 20, 400], 100, GAP, 40), 2)
check('gap 为 0 也算得对', fitTagCount([10, 10, 10], 30, 0, 5), 3)

// 不变量：结果落在 [1, 个数] 之间，而且说「放得下」就一定真的没超（超了就是折行）。
// 唯一的例外是「连一个都放不下」的兜底值 1——那时由 el-select 给标签挂 max-width
// 打省略号，宽度不再由内容决定。
{
  const cases = [
    { widths: TAGS, available: 33 },
    { widths: TAGS, available: 120 },
    { widths: [7, 900, 12], available: 300 },
    { widths: [50, 50], available: 0 },
    { widths: TAGS, available: 500 },
  ]
  const problems = []
  for (const { widths, available } of cases) {
    const got = fitTagCount(widths, available, GAP, 45)
    if (got < 1 || got > widths.length) problems.push(`结果越界：available=${available} got=${got}`)
    const used = widths.slice(0, got).reduce((sum, width) => sum + width, 0) + GAP * (got - 1)
    const floor = got === 1 && used + GAP + 45 > available
    if (used > available && !floor) {
      problems.push(`说放得下 ${got} 个，实际要 ${used}px > ${available}px`)
    }
  }
  check('结果不越界，且放得下就真的没超宽', problems, [])
}

console.log(failed ? `\n${failed} 项不通过` : '\n多选标签排版检查全部通过')
process.exit(failed ? 1 : 0)
