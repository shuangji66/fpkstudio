<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="flex shrink-0 items-center gap-2 border-b border-slate-200 bg-slate-50 px-3 py-1">
      <span class="text-[11px] text-slate-500">{{ t('cmdEditor.hint') }}</span>
      <span class="flex-1" />
      <el-button text size="small" :disabled="!props.path.endsWith('main')" @click="insertMainTemplate">
        {{ t('cmdEditor.template') }}
      </el-button>
    </div>
    <SourceEditor
      ref="inner"
      class="min-h-0 flex-1"
      :path="props.path"
      :origin="props.origin"
      :original="props.original"
      @dirty="(value: boolean) => emit('dirty', value)"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import SourceEditor from './SourceEditor.vue'
import type { EditorApi, EditorContent, EditorProps } from '../useEditor'

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()

const inner = ref<InstanceType<typeof SourceEditor> | null>(null)

/**
 * cmd/main 的骨架。
 *
 * 官方 fnpack 模板里的那份脚本本身有 bug（stop_process 里在函数外 `return`、
 * `local count` 用在函数外），照抄装上去会在 stop 时报错。这里给一份能跑的。
 */
const MAIN_TEMPLATE = `#!/bin/bash

LOG_FILE="\${TRIM_PKGVAR}/app.log"
PID_FILE="\${TRIM_PKGVAR}/app.pid"
APP_DIR="\${TRIM_APPDEST}/server"

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> "\${LOG_FILE}"
}

is_running() {
    [ -f "\${PID_FILE}" ] || return 1
    local pid
    pid=$(tr -d '[:space:]' < "\${PID_FILE}")
    [ -n "\${pid}" ] && kill -0 "\${pid}" 2>/dev/null
}

start_app() {
    is_running && return 0
    log "starting \${TRIM_APPNAME} \${TRIM_APPVER}"
    cd "\${APP_DIR}" || exit 1
    # TODO 改成你的启动命令
    # ./myapp --port "\${TRIM_SERVICE_PORT}" >> "\${LOG_FILE}" 2>&1 &
    echo $! > "\${PID_FILE}"
}

stop_app() {
    is_running || { rm -f "\${PID_FILE}"; return 0; }
    local pid
    pid=$(tr -d '[:space:]' < "\${PID_FILE}")
    log "stopping \${pid}"
    kill -TERM "\${pid}" 2>/dev/null

    local waited=0
    while kill -0 "\${pid}" 2>/dev/null && [ "\${waited}" -lt 10 ]; do
        sleep 1
        waited=$((waited + 1))
    done
    kill -0 "\${pid}" 2>/dev/null && kill -KILL "\${pid}" 2>/dev/null
    rm -f "\${PID_FILE}"
}

case "$1" in
    start) start_app ;;
    stop) stop_app ;;
    status) is_running && exit 0 || exit 3 ;;
    *) echo "unknown command: $1" > "\${TRIM_TEMP_LOGFILE}"; exit 1 ;;
esac
`

async function insertMainTemplate() {
  const text = inner.value?.getText?.() ?? ''
  if (text.trim()) {
    try {
      await ElMessageBox.confirm(
        '当前内容会被覆盖，确定插入模板吗？',
        t('common.tip'),
        { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
      )
    } catch {
      return
    }
  }
  inner.value?.setContent?.(MAIN_TEMPLATE)
  emit('dirty', true)
  ElMessage.success(t('common.ok'))
}

defineExpose({
  isDirty: (): boolean => inner.value?.isDirty?.() ?? false,
  getContent: (): EditorContent | null => inner.value?.getContent?.() ?? null,
  reset: () => inner.value?.reset?.(),
} satisfies EditorApi)
</script>
