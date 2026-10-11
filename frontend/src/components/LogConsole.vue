<template>
  <section
    class="shrink-0 border-t border-slate-300 bg-slate-900 text-slate-200 transition-[height] duration-200"
    :style="{ height: logs.panelOpen ? `${height}px` : '2rem' }"
  >
    <div class="flex h-8 items-center gap-2 px-2 text-xs">
      <button
        type="button"
        class="flex items-center gap-1 rounded px-1.5 py-0.5 hover:bg-white/10"
        @click="logs.panelOpen = !logs.panelOpen"
      >
        <el-icon><ArrowDown v-if="logs.panelOpen" /><ArrowUp v-else /></el-icon>
        {{ t('log.title') }}
      </button>

      <el-select
        v-model="logs.sourceFilter"
        size="small"
        class="w-[7.5rem]"
        :teleported="false"
      >
        <el-option :label="t('log.all')" value="" />
        <el-option :label="t('log.app')" value="app" />
        <el-option :label="t('log.fnpack')" value="fnpack" />
        <el-option :label="t('log.build')" value="build" />
      </el-select>

      <el-input
        v-model="logs.textFilter"
        size="small"
        class="w-[12rem]"
        clearable
        :placeholder="t('log.filterText')"
      />

      <span class="flex items-center gap-1 text-[11px] text-slate-400">
        <span
          class="inline-block h-1.5 w-1.5 rounded-full"
          :class="logs.connected ? 'bg-emerald-400' : 'bg-slate-500'"
        />
        {{ logs.connected ? t('log.connected') : t('log.disconnected') }}
      </span>

      <span class="text-[11px] text-slate-500">{{ logs.visible.length }} 条</span>

      <span class="flex-1" />

      <button
        type="button"
        class="rounded px-1.5 py-0.5 hover:bg-white/10"
        @click="clear"
      >
        {{ t('log.clear') }}
      </button>
    </div>

    <div
      v-show="logs.panelOpen"
      ref="host"
      class="term-host h-[calc(100%-2rem)] overflow-hidden"
    />
    <div
      v-if="logs.panelOpen"
      class="absolute right-0 top-0 h-1.5 w-full cursor-row-resize"
      @pointerdown="startResize"
    />
  </section>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { useLogsStore } from '@/stores/logs'
import type { LogEntry } from '@/api/types'

const { t } = useI18n()
const logs = useLogsStore()

const host = ref<HTMLElement | null>(null)
const height = ref(240)
let term: Terminal | null = null
let fit: FitAddon | null = null
let written = 0
let observer: ResizeObserver | null = null

const COLORS: Record<string, string> = {
  debug: '\x1b[90m',
  info: '\x1b[0m',
  warn: '\x1b[33m',
  error: '\x1b[31m',
}
const RESET = '\x1b[0m'
const DIM = '\x1b[90m'

function render(entry: LogEntry) {
  if (!term) return
  const color = COLORS[entry.level] ?? ''
  term.write(`${DIM}${entry.time}${RESET} ${color}${entry.msg}${RESET}\r\n`)
}

/** 只增量补写新条目，避免每来一条就重排整屏。 */
function flush() {
  if (!term) return
  const list = logs.visible
  if (written > list.length) {
    // 过滤条件变了，整屏重画。
    term.clear()
    term.write('\x1bc')
    written = 0
  }
  for (let i = written; i < list.length; i += 1) render(list[i]!)
  written = list.length
  term.scrollToBottom()
}

watch(() => logs.visible.length, () => flush())
watch(() => [logs.sourceFilter, logs.textFilter], () => {
  written = 0
  term?.reset()
  flush()
})
watch(
  () => logs.panelOpen,
  async (open) => {
    if (!open) return
    await nextTick()
    fit?.fit()
    flush()
  },
)

function clear() {
  logs.clear()
  written = 0
  term?.reset()
}

function startResize(event: PointerEvent) {
  const startY = event.clientY
  const startHeight = height.value
  const move = (e: PointerEvent) => {
    height.value = Math.min(Math.max(startHeight - (e.clientY - startY), 120), 640)
  }
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    void nextTick(() => fit?.fit())
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

onMounted(() => {
  if (!host.value) return
  term = new Terminal({
    convertEol: false,
    cursorBlink: false,
    disableStdin: true,
    fontSize: 12,
    fontFamily: 'var(--font-mono)',
    scrollback: 5000,
    theme: { background: '#0f172a', foreground: '#e2e8f0' },
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.open(host.value)
  fit.fit()

  observer = new ResizeObserver(() => fit?.fit())
  observer.observe(host.value)

  logs.connect()
  void nextTick(() => flush())
})

onUnmounted(() => {
  observer?.disconnect()
  term?.dispose()
  term = null
})
</script>
