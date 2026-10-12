import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getLogs, logStreamURL } from '@/api/endpoints'
import type { LogEntry } from '@/api/types'

const MAX_ENTRIES = 4000

/**
 * 日志与构建输出。
 *
 * 后端把所有输出写进同一个带 seq 的日志流，这里用 SSE 增量接收，按 seq 去重。
 * 去重必须知道「seq 属于哪个后端进程」：seq 是进程内计数，重启后从 1 重新开始，而
 * EventSource 自带重连（用的还是旧的 after），所以后端在流的第一帧报一个 boot 标识
 * （见 logging.Logger.boot）。不带它的话，重启之后的新日志会被当成「比游标旧」整片
 * 丢掉——表现是日志弹窗从此不再更新，要等新进程写到超过旧的最大值才恢复。
 */
export const useLogsStore = defineStore('logs', () => {
  const entries = ref<LogEntry[]>([])
  const connected = ref(false)
  const sourceFilter = ref<string>('')
  const textFilter = ref('')
  /** 日志 / 构建输出弹窗是否打开。 */
  const dialogOpen = ref(false)

  let stream: EventSource | null = null
  let lastSeq = 0
  /** 当前这批日志属于哪个后端进程（见 stores/logs.ts 的文件头说明）。 */
  let boot = ''

  /** 追加一条，并按上限裁掉最旧的几条。 */
  function appendEntry(entry: LogEntry) {
    entries.value.push(entry)
    if (entries.value.length > MAX_ENTRIES) {
      entries.value.splice(0, entries.value.length - MAX_ENTRIES)
    }
  }

  function push(entry: LogEntry) {
    if (entry.seq <= lastSeq) return
    lastSeq = entry.seq
    appendEntry(entry)
  }

  /**
   * 按 seq 合并一批历史，而不是整体覆盖。
   *
   * 补历史（`GET /api/logs`）和 SSE 的补发是并发跑的，谁后到不该决定内容：覆盖会把
   * 已经实时收到的行丢掉，还会把 lastSeq 往回拨，之后重连时重复收一遍。合并只按 seq
   * 去重、排序，两边谁先到结果都一样。
   */
  function mergeHistory(list: LogEntry[]) {
    if (!list.length) return
    const known = new Set(entries.value.map((e) => e.seq))
    const fresh = list.filter((e) => !known.has(e.seq))
    if (!fresh.length) return
    entries.value = [...entries.value, ...fresh]
      .sort((a, b) => a.seq - b.seq)
      .slice(-MAX_ENTRIES)
  }

  /** 后端换进程了：清掉旧进程的日志与游标（新进程的 seq 从 1 开始）。 */
  function resetForBoot(id: string): boolean {
    if (!id || id === boot) return false
    boot = id
    entries.value = []
    lastSeq = 0
    return true
  }

  async function loadHistory() {
    try {
      const res = await getLogs(0, 2000)
      if (resetForBoot(res.boot)) {
        // 换了进程：这份历史就是全部内容，游标取它的最大值。
        entries.value = res.list.slice(-MAX_ENTRIES)
        lastSeq = res.list.length ? res.list[res.list.length - 1]!.seq : 0
        return
      }
      mergeHistory(res.list)
      if (res.list.length) {
        lastSeq = Math.max(lastSeq, res.list[res.list.length - 1]!.seq)
      }
    } catch {
      /* 拿不到历史不影响实时日志 */
    }
  }

  function connect() {
    if (stream) return
    if (!entries.value.length) void loadHistory()

    stream = new EventSource(logStreamURL(lastSeq))
    stream.onopen = () => {
      connected.value = true
    }
    stream.onerror = () => {
      connected.value = false
      // EventSource 自带重连（同一个 URL、同一个 after）。真重启过的后端会在重连后
      // 用 boot 帧告诉我们「换进程了」，那时游标归零、再补一次历史。
    }
    // 进程标识帧：见文件头。换进程就要把游标归零，否则新日志全被当成旧的丢掉。
    stream.addEventListener('boot', (ev) => {
      let id = ''
      try {
        id = String((JSON.parse((ev as MessageEvent).data) as { boot?: string }).boot ?? '')
      } catch {
        /* 坏帧忽略：宁可这帧不用，也不能让流挂掉 */
      }
      if (resetForBoot(id)) void loadHistory()
    })
    stream.onmessage = (ev) => {
      try {
        push(JSON.parse(ev.data) as LogEntry)
      } catch {
        /* 忽略坏帧 */
      }
    }
  }

  function disconnect() {
    stream?.close()
    stream = null
    connected.value = false
  }

  function clear() {
    entries.value = []
  }

  const visible = computed(() => {
    let list = entries.value
    if (sourceFilter.value) list = list.filter((e) => e.source === sourceFilter.value)
    if (textFilter.value) {
      const needle = textFilter.value.toLowerCase()
      list = list.filter((e) => e.msg.toLowerCase().includes(needle))
    }
    return list
  })

  return {
    entries,
    connected,
    sourceFilter,
    textFilter,
    dialogOpen,
    visible,
    connect,
    disconnect,
    clear,
    push,
    // 补历史。boot 帧换进程时会调它；单独暴露出来是为了让 check:logs 能验
    // 「补历史和实时帧抢跑也不丢不重」这一条（列表本身不需要别的入口）。
    loadHistory,
  }
})
