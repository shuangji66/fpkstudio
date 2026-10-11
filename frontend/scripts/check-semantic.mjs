/**
 * 校验语义路径的整理逻辑（src/utils/semantic.ts）。
 *
 *   npm run check:semantic
 *
 * 它错了的表现是「设置弹窗里目录那一行退回显示 /vol1/...」，而且不报错——
 * 看起来像系统没返回语义路径，其实是这里没对上号。
 */
import { indexSemantics } from '../src/utils/semantic.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(
    `${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`,
  )
}

const item = (path, semanticPath) => ({ path, semanticPath })

check(
  '顺序一致时按路径对应',
  indexSemantics(['/vol1/a', '/vol1/b'], [item('/vol1/a', '存储空间1/a'), item('/vol1/b', '存储空间1/b')]),
  { '/vol1/a': '存储空间1/a', '/vol1/b': '存储空间1/b' },
)

// 系统回了多余的尾斜杠（真实系统上出现过这种事），不能因此整行退化成原始路径
check(
  '尾斜杠不影响对应',
  indexSemantics(['/vol1/a'], [item('/vol1/a/', '存储空间1/a')]),
  { '/vol1/a': '存储空间1/a' },
)

check(
  '顺序被打乱也按路径对应',
  indexSemantics(['/vol1/a', '/vol1/b'], [item('/vol1/b', '存储空间1/b'), item('/vol1/a', '存储空间1/a')]),
  { '/vol1/a': '存储空间1/a', '/vol1/b': '存储空间1/b' },
)

// 路径对不上（比如系统做了符号链接解析）时按位置兜底：文档保证 result 与请求一一对应
check(
  '路径对不上时按位置兜底',
  indexSemantics(['/vol1/link'], [item('/vol1/real', '存储空间1/re')]),
  { '/vol1/link': '存储空间1/re' },
)

check('空语义路径当作没有', indexSemantics(['/vol1/a'], [item('/vol1/a', '')]), {})
check('系统回空列表时什么都不给', indexSemantics(['/vol1/a'], []), {})
check('没请求就不查', indexSemantics([], [item('/vol1/a', '存储空间1/a')]), {})

// 根目录的尾斜杠不能归一成空串
check('根目录归一成 /', indexSemantics(['/v'], [item('////', '/')]), { '/v': '/' })

console.log(failed ? `\n${failed} 项不通过` : '\n语义路径检查全部通过')
process.exit(failed ? 1 : 0)
