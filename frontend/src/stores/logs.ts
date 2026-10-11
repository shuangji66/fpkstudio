import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getLogs, logStreamURL } from '@/api/endpoints'
import type { LogEntry } from '@/api/types'

const MAX_ENTRIES = 4000

/**
 * 日志与构建输出。
 *
 * 后端把所有输出写进同一个带 seq 的日志流，这里用 SSE 增量接收；
 * 断线重连后按 seq 去重，所以既不会丢也不会重复。
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

  function push(entry: LogEntry) {
    if (entry.seq <= lastSeq) return
    lastSeq = entry.seq
    entries.value.push(entry)
    if (entries.value.length > MAX_ENTRIES) {
      entries.value.splice(0, entries.value.length - MAX_ENTRIES)
    }
  }

  async function loadHistory() {
    try {
      const res = await getLogs(0, 2000)
      entries.value = res.list
      lastSeq = res.list.length ? res.list[res.list.length - 1]!.seq : 0
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
      // EventSource 自带重连，这里只更新状态；重连后会重发历史，
      // push() 按 seq 去重。
    }
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
  }
})
