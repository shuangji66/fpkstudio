/**
 * 路径 / 命名 / 文本格式的小工具（src/utils/format.ts、src/utils/validate.ts）。
 *
 *   npm run check:format
 *
 * 这些都是「算错了很难看出来」的那类：uniqueName 算错的表现是粘贴把已有的文件
 * 覆盖掉；isArchive 算错的表现是菜单里给了一个点了就报错的「解压」；formatTime
 * 算错则只是列表里的时间变得莫名其妙；checkFormat 判错的表现是**源码视图里一份
 * 写坏的配置被直接写进文件**（fnOS 那边直接失效），或者反过来把好好的文件拦住
 * 不让存。都不适合只靠手点。
 */
import { formatTime, isArchive, uniqueName } from '../src/utils/format.ts'
import { I18N_LOCALE_TEMPLATE } from '../src/utils/i18nFiles.ts'
import { Manifest } from '../src/utils/manifest.ts'
import { checkFormat } from '../src/utils/validate.ts'

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

// 能不能解压的判据必须和后端 fsx.ArchiveKindOf 一致（少一个就是「菜单里没有解压」）
for (const name of [
  'app.tar',
  'app.tar.gz',
  'app.tgz',
  'app.tar.bz2',
  'app.tbz2',
  'app.tbz',
  'app.tar.xz',
  'app.txz',
  'app.tar.zst',
  'app.tzst',
  'app.zip',
  'data.gz',
  'data.bz2',
  'data.xz',
  'data.zst',
]) {
  check(`${name} 认得出`, isArchive(name), true)
}
check('大小写不敏感', isArchive('APP.TAR.GZ') && isArchive('APP.ZIP'), true)
for (const name of ['app.7z', 'app.rar', 'app.tar.Z', 'mytar', 'manifest', 'a.gz.txt']) {
  check(`${name} 不认`, isArchive(name), false)
}

// 修改时间的展示格式（参照时刻固定，不然测试会随日期变）
{
  const now = new Date('2026-07-11T20:00:00')
  check('今天的只留时分', formatTime('2026-07-11 09:05:00', now), '09:05')
  check('今年的省掉年份', formatTime('2026-03-02 09:05:00', now), '03-02 09:05')
  check('往年的只留日期', formatTime('2025-03-02 09:05:00', now), '2025-03-02')
  check('解析不了就原样返回', formatTime('—', now), '—')
}

// 源码视图保存前的格式校验：该拦的拦住，不该拦的别拦
check('manifest 正常不报', checkFormat('manifest', 'appname = demo\nversion = 1.0.0\n'), null)
check('manifest 注释和空行不算错', checkFormat('manifest', '# 说明\n\nappname = demo\n'), null)
check(
  'manifest 多行值里的续行不算错',
  checkFormat('manifest', 'desc = """第一行\n第二行"""\nappname = demo\n'),
  null,
)
check(
  'manifest 缺等号的行要报',
  checkFormat('manifest', 'appname = demo\noops\n')?.key,
  'editor.formatManifest',
)
check('privilege 的坏 JSON 要报', checkFormat('config/privilege', '{"run-as":}')?.key, 'editor.formatJson')
check('privilege 的好 JSON 不报', checkFormat('config/privilege', '{"run-as":"package"}'), null)
check('入口配置的坏 JSON 要报', checkFormat('app/ui/config', '{oops}')?.key, 'editor.formatJson')
// i18n 的 locale 文件是 INI（官方语法），**不是 JSON**：以前这边把它和几份 JSON 配置
// 归成一类，判据完全对不上——一份合法的 locale 文件在源码视图里存不进去，而它只有
// 源码视图这一条路（registry 里没有 i18n 编辑器）。这里钉住「合法的放行、会静默失效的拦住」。
check('i18n 的空模板不报', checkFormat('i18n/en-US', I18N_LOCALE_TEMPLATE), null)
check(
  'i18n 的合法 INI 不报（注释、空行、section、带引号的值）',
  checkFormat('i18n/zh-CN', '# 说明\n\n[common]\ndisplayName="演示"\n'),
  null,
)
check('i18n 的不带引号的值也放行', checkFormat('i18n/zh-CN', '[common]\ndesc=hello\n'), null)
check(
  'i18n 里既不是 section 也不是 key=值 的行要报',
  checkFormat('i18n/zh-CN', '[common]\n{oops}\n')?.key,
  'editor.formatI18n',
)
check(
  'i18n 的引号没闭合要报',
  checkFormat('i18n/zh-CN', '[common]\ndisplayName="演示\n')?.key,
  'editor.formatI18nQuote',
)
check('i18n 文件不再按 JSON 判', checkFormat('i18n/zh-CN', 'not json at all')?.key, 'editor.formatI18n')
check('向导的坏 JSON 要报', checkFormat('wizard/install', '[{]')?.key, 'editor.formatWizard')
check('向导的空数组不报', checkFormat('wizard/install', '[]'), null)
check('向导的空文件不报', checkFormat('wizard/install', ''), null)
check('生命周期脚本不校验', checkFormat('cmd/main', 'if [ -z "$x" ]; then'), null)
check('普通文本不校验', checkFormat('notes.txt', '随便写点什么 {'), null)

// manifest 的回写：**没编辑过的内容必须原样回写**（对齐、缩进、注释、多行值），
// 只有被改过的值才允许变。这是 manifest.ts 存在的理由（「打开一次保存后整个文件
// 被重排」是它要避免的事），所以拿一份写得很别扭的模板钉住它。
{
  const src = [
    '# 打包说明',
    'appname               = demo',
    'version               = 1.0.0',
    'desc                  = """第一行',
    '第二行"""',
    '  platform = x86',
    '',
  ].join('\n')

  const untouched = Manifest.parse(src)
  check('未编辑时原样回写', untouched.toText(), src)

  const edited = Manifest.parse(src)
  edited.set('version', '2.0.0')
  check('只改动被编辑的那个键', edited.toText(), src.replace('= 1.0.0', '= 2.0.0'))

  const added = Manifest.parse(src)
  added.set('maintainer', 'me')
  check('新键插在末尾空行之前', added.toText(), src + 'maintainer = me\n')

  const removed = Manifest.parse(src)
  removed.remove('desc')
  check('删键只删那一行', removed.toText().includes('desc'), false)

  // 已知取舍：值的写法会被规范化（引号 / 行尾空格不留），值本身不变。
  check(
    '值的引号会被规范化',
    Manifest.parse('version = "1.0.0"   \n').toText(),
    'version = 1.0.0\n',
  )
}

console.log(failed ? `\n${failed} 项不通过` : '\n路径 / 命名 / 文本格式检查全部通过')
process.exit(failed ? 1 : 0)
