/**
 * 文件管理器的 store（src/stores/workspace.ts）里「会丢 / 写坏用户文件」的那几条路径。
 *
 *   npm run check:workspace
 *
 * 盯三件事，都是踩过的：
 *  1. **重命名一个打开着的文件**：标签要跟着改名，而 `origin` / `dirty` / `draft` 一个
 *     都不能丢。编辑器宿主是按 `file.path` 做 key 的，key 一变组件就重建、内容只能从
 *     `draft || origin` 里取——清掉 origin 的表现是「改个文件名，编辑框空了，随手敲一个
 *     字保存，文件就只剩那一个字」。
 *  2. **重命名目录**：目录里已经打开的文件标签路径要一起改，否则保存会照着旧路径写回去，
 *     凭空「复活」一个文件（后端写文件会顺手建父目录）。
 *  3. **重名 / 非法名字要拒绝**：rename(2) 会直接覆盖同名文件，用户只是想改个名。
 *
 * 跑的是真实的 store：window / localStorage / fetch 自己搭，后端用一个内存文件表模拟。
 * 不是集成测试，只是把上面几条判据钉住，免得以后又被改回去。
 */
import { createPinia, setActivePinia } from 'pinia'

globalThis.window = {
  localStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  addEventListener: () => {},
  matchMedia: () => ({ matches: false, addEventListener: () => {} }),
  visualViewport: null,
}

/** 内存里的假后端：路径 → 内容。 */
const files = new Map([
  ['demo/notes.txt', 'ORIGINAL CONTENT\n'],
  ['demo/other.txt', 'OTHER\n'],
  ['demo/dir/inner.txt', 'INNER\n'],
])
const calls = []

globalThis.fetch = async (url, init = {}) => {
  const route = String(url).split('?')[0]
  const body = init.body ? JSON.parse(init.body) : {}
  calls.push({ route, body })
  const json = (value, status = 200) =>
    new Response(JSON.stringify(value), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })

  if (route.endsWith('/api/fs/list')) {
    const dir = body.path
    const entries = []
    const push = (name, isDir) => {
      if (!entries.some((entry) => entry.name === name)) {
        entries.push({ name, dir: isDir, size: 0, mode: '', modTime: '2026-01-01 00:00:00', symlink: false })
      }
    }
    for (const path of files.keys()) {
      if (!path.startsWith(`${dir}/`)) continue
      const rest = path.slice(dir.length + 1)
      const cut = rest.indexOf('/')
      if (cut < 0) push(rest, false)
      else push(rest.slice(0, cut), true)
    }
    return json({ path: dir, abs: `/${dir}`, entries })
  }
  if (route.endsWith('/api/fs/read')) {
    const content = files.get(body.path)
    if (content === undefined) return json({ error: 'not found' }, 404)
    return json({ path: body.path, size: content.length, sizeRaw: '1 B', text: true, content })
  }
  if (route.endsWith('/api/fs/write')) {
    files.set(body.path, body.content)
    return json({ ok: true, path: body.path, size: body.content.length })
  }
  if (route.endsWith('/api/fs/rename')) {
    if (files.has(body.path)) {
      files.set(body.to, files.get(body.path))
      files.delete(body.path)
    } else {
      // 目录：前缀下的条目一起搬（先快照 key，免得边改边遍历）
      const prefix = `${body.path}/`
      const moving = [...files.keys()].filter((path) => path.startsWith(prefix))
      if (!moving.length) return json({ error: 'not found' }, 404)
      for (const path of moving) {
        files.set(body.to + path.slice(body.path.length), files.get(path))
        files.delete(path)
      }
    }
    return json({ ok: true, path: body.to })
  }
  if (route.endsWith('/api/projects/exists')) return json({ exists: true, docker: false, ui: false })
  if (route.endsWith('/api/projects/platforms')) return json({ platforms: [] })
  if (route.endsWith('/api/projects/settings')) {
    return json({ multiPlatform: false, platforms: [], serverDir: 'app/server', copyToOutput: true })
  }
  if (route.endsWith('/api/projects/artifacts')) return json({ list: [], outputDir: '/out' })
  return json({})
}

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(
    `${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`,
  )
}
/** 断言某个异步调用被拒绝（reject 的原因只打印出来，不做文案比对）。 */
async function rejects(name, run) {
  try {
    await run()
    failed++
    console.log(`FAIL ${name}（没有拒绝）`)
  } catch {
    console.log(`ok   ${name}`)
  }
}

setActivePinia(createPinia())
const { useWorkspaceStore } = await import('@/stores/workspace.ts')
const workspace = useWorkspaceStore()
const tabOf = (project, path) => workspace.state(project).files.find((f) => f.path === path)

// ---- 1. 重命名一个打开着的文件 ----
{
  const tab = await workspace.openFile('demo', 'demo/notes.txt')
  check('打开文件读到内容', tab.origin, 'ORIGINAL CONTENT\n')

  // 模拟编辑器里改了还没保存
  tab.dirty = true
  tab.draft = 'EDITED BUT UNSAVED\n'

  await workspace.renameEntryAt('demo', 'demo/notes.txt', 'demo/renamed.txt')
  const renamed = tabOf('demo', 'renamed.txt')
  check('改名后标签跟着换路径', renamed?.path, 'renamed.txt')
  check('改名后内容基线还在', renamed?.origin, 'ORIGINAL CONTENT\n')
  check('改名后脏标记还在', renamed?.dirty, true)
  check('改名后草稿还在', renamed?.draft, 'EDITED BUT UNSAVED\n')
  check('改名后 activeTab 指向新路径', workspace.state('demo').activeTab, 'renamed.txt')
  check('改名后磁盘上换了名字', files.has('demo/renamed.txt') && !files.has('demo/notes.txt'), true)

  // 接着保存：内容落在新路径上
  await workspace.saveFile('demo', 'renamed.txt', renamed.draft)
  check('保存写进新路径', files.get('demo/renamed.txt'), 'EDITED BUT UNSAVED\n')
  check('保存后不再是脏的', tabOf('demo', 'renamed.txt').dirty, false)
}

// ---- 2. 重命名目录：里面的子标签要跟着改 ----
{
  const inner = await workspace.openFile('demo', 'demo/dir/inner.txt')
  check('打开目录里的文件', inner.origin, 'INNER\n')

  await workspace.renameEntryAt('demo', 'demo/dir', 'demo/dir2')
  const moved = tabOf('demo', 'dir2/inner.txt')
  check('目录改名后子标签换路径', moved?.path, 'dir2/inner.txt')
  check('目录改名后内容基线还在', moved?.origin, 'INNER\n')

  calls.length = 0
  await workspace.saveFile('demo', moved.path, 'SAVED\n')
  const written = calls.filter((c) => c.route.endsWith('/api/fs/write')).at(-1)
  check('保存落在改名后的目录里', written?.body.path, 'demo/dir2/inner.txt')
  check('旧路径没有被复活', files.has('demo/dir/inner.txt'), false)
}

// ---- 3. 重名 / 非法名字要拒绝，而且什么都不改 ----
{
  calls.length = 0
  await rejects('重名时拒绝改名', () =>
    workspace.renameEntryAt('demo', 'demo/renamed.txt', 'demo/other.txt'),
  )
  // 判重名要先现拉一次目录（force），所以允许有 list，但绝不允许有写请求
  const writes = () => calls.filter((c) => !c.route.endsWith('/api/fs/list')).length
  check('重名被拒时没有发写请求', writes(), 0)
  check('重名被拒时两个文件都还在', [files.has('demo/renamed.txt'), files.has('demo/other.txt')], [true, true])

  await rejects('名字里带分隔符要拒绝', () =>
    workspace.renameEntryAt('demo', 'demo/renamed.txt', 'demo/sub/renamed.txt'),
  )
  await rejects('空名字要拒绝', () => workspace.renameEntryAt('demo', 'demo/renamed.txt', 'demo/'))
  check('非法名字之后没有发写请求', writes(), 0)
  check('非法名字之后文件还在', files.has('demo/renamed.txt'), true)
}

// ---- 4. 新建文件不许覆盖已有文件 ----
{
  calls.length = 0
  await rejects('同名文件要拒绝新建', () =>
    workspace.createEntryAt('demo', 'demo', 'other.txt', false),
  )
  const writes = calls.filter((c) => !c.route.endsWith('/api/fs/list')).length
  check('同名新建被拒时没有发写请求', writes, 0)
  check('同名新建被拒时文件内容没变', files.get('demo/other.txt'), 'OTHER\n')
}

// ---- 5. 目录改名 / 删除之后：活动标签、目录缓存、剪贴板都要跟着走 ----
{
  await workspace.loadDir('demo', 'demo/dir2')
  workspace.state('demo').expanded['demo/dir2'] = true

  const inner = await workspace.openFile('demo', 'demo/dir2/inner.txt')
  check('打开改名前的子文件', inner.path, 'dir2/inner.txt')
  workspace.setActive('demo', 'dir2/inner.txt')

  await workspace.renameEntryAt('demo', 'demo/dir2', 'demo/dir3')
  // 活动的是「被改名目录里的子文件」：activeTab 不跟着换的话，ProjectTab 里所有
  // v-show="activeTab === …" 都不成立，编辑区整片空白。
  check('目录改名后 activeTab 跟着换', workspace.state('demo').activeTab, 'dir3/inner.txt')
  check('目录改名后缓存键搬到新路径', workspace.state('demo').dirs['demo/dir3'] !== undefined, true)
  check('目录改名后旧缓存键清掉', workspace.state('demo').dirs['demo/dir2'], undefined)
  check('目录改名后展开状态跟着搬', workspace.state('demo').expanded['demo/dir3'], true)

  // 剪贴板指向被改名的项：路径要跟着换，否则之后每次粘贴都用旧路径
  workspace.copyToClipboard('demo', 'demo/dir3/inner.txt', true)
  await workspace.renameEntryAt('demo', 'demo/dir3/inner.txt', 'demo/dir3/renamed.txt')
  // 剪贴板里存的是「含项目名的完整路径」（和 FileTree / FileBrowser 传进来的一致）
  check('重命名后剪贴板路径跟着换', workspace.state('demo').clipboard?.path, 'demo/dir3/renamed.txt')

  // 删掉目录：缓存、展开状态、剪贴板里那条（在删除的子树里）都要清干净
  await workspace.removeEntryAt('demo', 'demo/dir3', true)
  check('删除目录后剪贴板被清掉', workspace.state('demo').clipboard, null)
  check('删除目录后没有残留的缓存键', workspace.state('demo').dirs['demo/dir3'], undefined)
  check('删除目录后没有残留的展开状态', workspace.state('demo').expanded['demo/dir3'], undefined)
  check('删除目录后子标签被关掉', tabOf('demo', 'dir3/renamed.txt'), undefined)
}

console.log(failed ? `\n${failed} 项不通过` : '\n文件管理器 store 检查全部通过')
process.exit(failed ? 1 : 0)
