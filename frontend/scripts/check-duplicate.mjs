/**
 * 「创建副本」的两处文本改写（src/utils/duplicate.ts），以及 manifest「默认入口」下拉
 * 的候选取值（src/utils/entries.ts，同一份 config 的另一种读法）。
 *
 *   npm run check:duplicate
 *
 * 这里算错不会报错，只会让副本「看起来正常但用不了」：入口 id 前缀没换，副本装上去
 * 桌面图标点不开；或者 manifest.appname 没换，副本和原项目抢同一个应用标识
 * （fsx.Copy 是复制目录，不会替我们改这些）。所以拿几份真实形状的文本钉住。
 */
import { replaceEntryPrefix, rewriteForDuplicate } from '../src/utils/duplicate.ts'
import { entryIds } from '../src/utils/entries.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`)
}

// ---- 入口 id 的前缀 ----
check('常规 id 换前缀', replaceEntryPrefix('myapp.main', 'copy'), 'copy.main')
check('只换第一个点之前那一段', replaceEntryPrefix('myapp.a.b', 'copy'), 'copy.a.b')
check('没有点就原样留着', replaceEntryPrefix('main', 'copy'), 'main')
check('点在最前面也原样留着', replaceEntryPrefix('.main', 'copy'), '.main')
check('空串原样留着', replaceEntryPrefix('', 'copy'), '')

// ---- manifest ----
const MANIFEST = `# 说明
appname        = myapp
version        = 1.0.0
display_name   = 我的应用
desktop_applaunchname = myapp.main
desc = """第一行
第二行"""
`

const rewritten = rewriteForDuplicate('copy', MANIFEST, null)
check('appname 换成副本名', /appname\s+= copy\n/.test(rewritten.manifest), true)
check('默认入口前缀跟着换', rewritten.manifest.includes('desktop_applaunchname = copy.main'), true)
check('版本号不动', rewritten.manifest.includes('version        = 1.0.0'), true)
check('对齐与注释原样保留', rewritten.manifest.startsWith('# 说明\nappname        = copy'), true)
check('多行值原样保留', rewritten.manifest.includes('desc = """第一行\n第二行"""'), true)
check('没有 config 时报 missing', rewritten.uiConfigSkipped, 'missing')
check('没有 config 时不写回', rewritten.uiConfig, null)

const noEntry = rewriteForDuplicate('copy', 'appname = demo\n', null)
check('没有 appname 时补一条', noEntry.manifest, 'appname = copy\n')
const noLaunch = rewriteForDuplicate('copy', 'appname = demo\ndesktop_applaunchname = main\n', null)
check('默认入口没有前缀就不动它', noLaunch.manifest.includes('desktop_applaunchname = main'), true)

// ---- app/ui/config ----
const CONFIG = JSON.stringify(
  {
    '.url': {
      'myapp.main': { title: '主页', icon: 'images/icon_{0}.png', type: 'iframe' },
      'myapp.Application': { title: '应用', type: 'url', url: '/' },
    },
    other: { keep: true },
  },
  null,
  2,
)

const withConfig = rewriteForDuplicate('copy', MANIFEST, CONFIG)
check('config 解析成功时不报跳过', withConfig.uiConfigSkipped, '')
const parsed = JSON.parse(withConfig.uiConfig)
check('入口 id 全部换了前缀', Object.keys(parsed['.url']), ['copy.Application', 'copy.main'])
check('入口里的内容没被动', parsed['.url']['copy.main'].title, '主页')
check('顶层其他键原样留着', parsed.other, { keep: true })
check('写回按字母序、2 空格、结尾带换行', withConfig.uiConfig.endsWith('}\n'), true)
check('默认入口和 config 里的 id 是同一套', /desktop_applaunchname = copy.main/.test(withConfig.manifest), true)

// 坏的 config：宁可不动，也不能拿解析出来的残缺内容盖掉用户原来的文本
const broken = rewriteForDuplicate('copy', MANIFEST, '{oops')
check('坏 config 报 invalid', broken.uiConfigSkipped, 'invalid')
check('坏 config 不写回', broken.uiConfig, null)
check('坏 config 时 manifest 照常改', /appname\s+= copy\n/.test(broken.manifest), true)

// 没有 .url 的 config（比如只有 docker-project）：也不用动
const noUrls = rewriteForDuplicate('copy', MANIFEST, '{"docker-project":{}}\n')
check('没有 .url 时不写回', noUrls.uiConfig, null)
check('没有 .url 时不算异常跳过', noUrls.uiConfigSkipped, '')

// ---- 入口 id 列表（manifest 的「默认入口」下拉） ----
check('取出全部入口 id', entryIds(CONFIG), ['myapp.main', 'myapp.Application'])
check('没有 .url 时没有候选', entryIds('{"docker-project":{}}'), [])
check('解析不了时没有候选（也不报错）', entryIds('{oops'), [])
check('.url 不是对象时没有候选', entryIds('{".url":[]}'), [])
check('空文本没有候选', entryIds(''), [])
check('顺序就是文件里的书写顺序（不排序）', entryIds('{".url":{"b.x":{},"a.y":{}}}'), ['b.x', 'a.y'])

console.log(failed ? `\n${failed} 项不通过` : '\n创建副本的文本改写检查全部通过')
process.exit(failed ? 1 : 0)
