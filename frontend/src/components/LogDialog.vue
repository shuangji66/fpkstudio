<template>
  <el-dialog
    v-model="logs.dialogOpen"
    :title="t('log.dialogTitle')"
    class="app-dialog"
    modal-class="app-modal"
    :width="layout.isMobile ? '95%' : '78%'"
    align-center
    lock-scroll
    append-to-body
    :close-on-click-modal="false"
    @open="onOpen"
    @opened="fitNow"
  >
    <div class="flex h-full min-h-0 flex-col">
      <div class="flex shrink-0 items-center gap-2 pb-2 text-sm">
        <el-select v-model="logs.sourceFilter" size="default" class="w-[7.5rem]" :teleported="false">
          <el-option :label="t('log.all')" value="" />
          <el-option :label="t('log.app')" value="app" />
          <el-option :label="t('log.fnpack')" value="fnpack" />
          <el-option :label="t('log.build')" value="build" />
        </el-select>

        <el-input
          v-model="logs.textFilter"
          size="default"
          class="w-[10rem]"
          clearable
          :placeholder="t('log.filterText')"
        />

        <span class="flex-1" />

        <button type="button" class="rounded px-1.5 py-0.5 text-slate-500 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-700" @click="clear">
          {{ t('log.clear') }}
        </button>
      </div>

      <div ref="host" class="term-host min-h-0 flex-1 overflow-hidden rounded-md bg-slate-900" />
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { nextTick, onUnmounted, useTemplateRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { useLayoutStore } from '@/stores/layout'
import { useLogsStore } from '@/stores/logs'
import type { LogEntry } from '@/api/types'

const { t } = useI18n()
const logs = useLogsStore()
const layout = useLayoutStore()

const host = useTemplateRef<HTMLDivElement>('host')

let term: Terminal | null = null
let fit: FitAddon | null = null
/** 已经写进终端的条数，用来做增量补写。 */
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

/**
 * 弹窗第一次打开时才建终端。
 *
 * 关闭状态下容器是 display:none，xterm 量不出行列数，所以既不能提前建，
 * 也不能在隐藏时 fit。
 */
function ensureTerm() {
  if (term || !host.value) return
  term = new Terminal({
    convertEol: false,
    cursorBlink: false,
    disableStdin: true,
    fontSize: 14,
    fontFamily: 'var(--font-mono)',
    scrollback: 5000,
    theme: { background: '#0f172a', foreground: '#e2e8f0' },
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.open(host.value)

  observer = new ResizeObserver(() => fitNow())
  observer.observe(host.value)
}

function fitNow() {
  if (logs.dialogOpen) fit?.fit()
}

async function onOpen() {
  await nextTick()
  ensureTerm()
  fitNow()
  flush()
}

watch(
  // 盯「最后一条的 seq」而不是长度：条目到上限（MAX_ENTRIES）之后 push + splice 让长度
  // 恒定，长度不变这个 watch 就再也不触发——终端不再画新行、右上角的条数也卡住（跑一次
  // 多架构打包很容易攒够几千行）。seq 一直在涨，永远能感知到新内容。
  () => logs.visible[logs.visible.length - 1]?.seq ?? 0,
  () => {
    if (logs.dialogOpen) flush()
  },
)
watch(
  () => [logs.sourceFilter, logs.textFilter],
  () => {
    written = 0
    term?.reset()
    if (logs.dialogOpen) flush()
  },
)

function clear() {
  logs.clear()
  written = 0
  term?.reset()
}

onUnmounted(() => {
  observer?.disconnect()
  term?.dispose()
  term = null
})
</script>
