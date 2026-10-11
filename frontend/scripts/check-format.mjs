/**
 * 校验路径与命名的小工具（src/utils/format.ts）。
 *
 *   npm run check:format
 *
 * 现在只有 uniqueName 一个：粘贴时靠它挑一个不重名的名字。它要是算错，
 * 表现是「粘贴把已有的文件覆盖掉」——这种事不能只靠手点来看。
 */
import { uniqueName } from '../src/utils/format.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`)
}

const names = (...list) => list.map((name) => ({ name }))

check('不重名时原样用工来的名字', uniqueName(names('b.txt'), 'a.txt', 'copy'), 'a.txt')
check('重名时加后缀，保留扩展名', uniqueName(names('a.txt'), 'a.txt', 'copy'), 'a - copy.txt')
check('后缀也重名就往后排', uniqueName(names('a.txt', 'a - copy.txt'), 'a.txt', 'copy'), 'a - copy2.txt')
check(
  '一路排到有空位',
  uniqueName(names('a.txt', 'a - copy.txt', 'a - copy2.txt', 'a - copy3.txt'), 'a.txt', 'copy'),
  'a - copy4.txt',
)
check('目录名（没有扩展名）直接加后缀', uniqueName(names('stuff'), 'stuff', 'copy'), 'stuff - copy')
check('点开头的隐藏文件当没有扩展名', uniqueName(names('.gitignore'), '.gitignore', 'copy'), '.gitignore - copy')
check('多个点的文件只切最后一段扩展名', uniqueName(names('a.tar.gz'), 'a.tar.gz', 'copy'), 'a.tar - copy.gz')

// 兜底不变量：不管已有多少同名，结果一定是个新名字
{
  const taken = names('a.txt', ...Array.from({ length: 6 }, (_, i) => `a - copy${i === 0 ? '' : i + 1}.txt`))
  const got = uniqueName(taken, 'a.txt', 'copy')
  check('结果一定不在已有名字里', taken.some((entry) => entry.name === got), false)
}

console.log(failed ? `\n${failed} 项不通过` : '\n路径与命名检查全部通过')
process.exit(failed ? 1 : 0)
