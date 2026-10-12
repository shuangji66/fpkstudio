/**
 * 校验日志 store 的去重与补历史（src/stores/logs.ts）。
 *
 *   npm run check:logs
 *
 * 盯三件事，都是「界面上看起来只是『日志不动了』、实际数据没坏」的那种故障：
 *  1. **后端重启**：日志的 seq 是**进程内**计数，重启从 1 开始，而 EventSource 自带
 *     重连（用的还是旧的 after）。前端如果只按 seq 去重、不认进程标识，新进程的日志会
 *     被整片丢掉——日志弹窗从此不再更新。这里用假 EventSource 驱动真实的 store 验它。
 *  2. **补历史与实时抢跑**：`GET /api/logs` 和 SSE 的补发是并发跑的，谁后到都不该
 *     改变内容（以前是整体覆盖 + 游标回拨，会把已经收到的行丢掉、之后重复收）。
 *  3. 到了条数上限（裁剪）之后仍然接收新条目。
 */
import { createPinia, setActivePinia } from 'pinia'

globalThis.window = {
  __FPKSTUDIO__: { baseURL: '/', version: 'test' },
  localStorage: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  addEventListener: () => {},
  matchMedia: () => ({ matches: false, addEventListener: () => {} }),
  visualViewport: null,
}

/** 假 EventSource：记录实例，手工投递 boot / message / error 帧。 */
class FakeEventSource {
  static instances = []

  /** 已建立的连接数（用于断言只连一次）。 */
  static get last() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]
  }

  constructor(url) {
    this.url = url
    this.closed = false
    this.listeners = {}
    FakeEventSource.instances.push(this)
  }

  addEventListener(type, fn) {
    ;(this.listeners[type] ??= []).push(fn)
  }

  close() {
    this.closed = true
  }

  /** 投递一个事件；data 会像浏览器那样被 JSON 序列化。 */
  emit(type, payload) {
    for (const fn of this.listeners[type] ?? []) fn({ data: JSON.stringify(payload) })
  }

  emitMessage(entry) {
    if (this.onmessage) this.onmessage({ data: JSON.stringify(entry) })
  }

  emitError() {
    if (this.onerror) this.onerror({})
  }
}
globalThis.EventSource = FakeEventSource

/** 假后端的 /api/logs 响应，按用例改写。 */
let history = { list: [], lastSeq: 0, boot: 'A' }

globalThis.fetch = async (url) => {
  const route = String(url).split('?')[0]
  const json = (value) =>
    new Response(JSON.stringify(value), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  if (route.endsWith('/api/logs')) {
    // 让补历史真的晚一拍返回：并发的实时帧先落地，才测得到「不覆盖」。
    await new Promise((r) => setTimeout(r, 0))
    return json(history)
  }
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

const entry = (seq, msg = `m${seq}`) => ({
  seq,
  time: '2026-01-01 00:00:00.000',
  level: 'info',
  source: 'app',
  msg,
})
const seqs = (list) => list.map((e) => e.seq)
/** 等几轮微任务 + 定时器，让 store 里的异步补历史跑完。 */
const settle = () => new Promise((r) => setTimeout(r, 5))

setActivePinia(createPinia())
const { useLogsStore } = await import('@/stores/logs.ts')
const logs = useLogsStore()

// ---- 1. 首次连接：boot 帧 + 补历史 ----
{
  history = { list: [entry(1), entry(2), entry(3)], lastSeq: 3, boot: 'A' }
  logs.connect()
  check('只建一条 SSE 连接', FakeEventSource.instances.length, 1)
  const es = FakeEventSource.last
  es.emit('boot', { boot: 'A', lastSeq: 3 })
  await settle()
  check('首次连接补上历史', seqs(logs.entries), [1, 2, 3])

  es.emitMessage(entry(4))
  await settle()
  check('实时帧接在历史后面', seqs(logs.entries), [1, 2, 3, 4])

  // 重连时后端会重发历史：同一条不许出现两次
  es.emitMessage(entry(4))
  es.emitMessage(entry(3))
  await settle()
  check('重复的 seq 被丢掉', seqs(logs.entries), [1, 2, 3, 4])
}

// ---- 2. 后端重启：seq 从 1 重新开始，必须把游标归零 ----
{
  const es = FakeEventSource.last
  // 重启前的游标是 4；新进程只带自己的 1、2 两条
  history = { list: [entry(1, 'new-a'), entry(2, 'new-b')], lastSeq: 2, boot: 'B' }
  es.emit('boot', { boot: 'B', lastSeq: 2 })
  await settle()
  check('换进程后清掉旧进程的日志', seqs(logs.entries), [1, 2])
  check('换进程后内容来自新进程', logs.entries[1].msg, 'new-b')

  // 关键：新进程的 seq（3）比旧游标（4）小，但必须被接受
  es.emitMessage(entry(3, 'new-c'))
  await settle()
  check('重启后新日志能进来（seq 比旧游标小也要收）', seqs(logs.entries), [1, 2, 3])

  // 同一条 boot 反复报（每次重连都会报）不该把日志清掉
  es.emit('boot', { boot: 'B', lastSeq: 3 })
  await settle()
  check('同一个进程的 boot 帧不清日志', seqs(logs.entries), [1, 2, 3])
}

// ---- 3. 补历史与实时抢跑：谁后到都不许丢行 / 不许重复 ----
{
  const es = FakeEventSource.last
  es.emitMessage(entry(50, 'live'))
  await settle()
  check('实时帧先到', seqs(logs.entries), [1, 2, 3, 50])

  // 补历史后到：里面包含 50 以及前面那些
  history = { list: [entry(1), entry(2), entry(3), entry(50)], lastSeq: 50, boot: 'B' }
  await logs.loadHistory()
  check('补历史不覆盖已经实时收到的行', seqs(logs.entries), [1, 2, 3, 50])
  check('补历史不产生重复', new Set(seqs(logs.entries)).size, logs.entries.length)

  // 补历史补上中间缺的那几条（seq 4..5 之前没收到）
  history = { list: [entry(4), entry(5), entry(50)], lastSeq: 50, boot: 'B' }
  await logs.loadHistory()
  check('缺的行被补上且顺序正确', seqs(logs.entries), [1, 2, 3, 4, 5, 50])
  // 游标不回退：50 之后的仍然收得到
  es.emitMessage(entry(51))
  await settle()
  check('补历史之后游标不回退', seqs(logs.entries), [1, 2, 3, 4, 5, 50, 51])
}

// ---- 4. 条数上限之后仍然接收新条目（弹窗的 watch 盯的就是最后一条的 seq）----
{
  for (let i = 52; i <= 4100; i += 1) logs.push(entry(i))
  check('长度封顶', logs.entries.length, 4000)
  check('最新的一条仍在最后', logs.entries[logs.entries.length - 1].seq, 4100)
  check('最旧的被裁掉', logs.entries.some((e) => e.seq === 1), false)
  logs.push(entry(4101))
  check('封顶之后仍然接新的', logs.entries[logs.entries.length - 1].seq, 4101)
}

// ---- 5. 断开 / 清空 ----
{
  logs.disconnect()
  check('断开后连接被关掉', FakeEventSource.last.closed, true)
  logs.clear()
  check('清空之后没有条目', logs.entries.length, 0)
  // 清空不清游标：否则断开重连时后端补发的旧行会「复活」。
  logs.push(entry(4102))
  check('清空之后仍然接收新条目', seqs(logs.entries), [4102])
  logs.push(entry(4101))
  check('清空之后旧的 seq 仍然被丢掉', seqs(logs.entries), [4102])
}

console.log(failed ? `\n${failed} 项不通过` : '\n日志 store 检查全部通过')
process.exit(failed ? 1 : 0)
