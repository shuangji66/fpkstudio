<template>
  <el-dialog
    v-model="visible"
    :title="t('envVars.title')"
    width="46rem"
    top="7vh"
    :close-on-click-modal="false"
  >
    <p class="mb-2 rounded-md bg-slate-50 px-3 py-2 text-[11px] leading-relaxed text-slate-500">
      {{ t('envVars.hint') }}
      <code class="mono rounded bg-white px-1 text-slate-700">${TRIM_APPNAME}</code>
    </p>

    <div v-if="loading" class="py-6 text-center text-xs text-slate-400">
      {{ t('common.loading') }}
    </div>

    <div v-else class="thin-scroll h-[60vh] space-y-3 overflow-y-auto pr-1">
      <section
        v-for="section in sections"
        :key="section.key"
        class="rounded-lg border border-slate-200"
      >
        <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5">
          <span class="text-xs font-medium text-slate-700">{{ t(section.labelKey) }}</span>
          <span class="flex-1" />
          <span class="text-[11px] text-slate-400">{{ section.rows.length }}</span>
        </header>
        <p v-if="section.noteKey" class="px-3 pt-1.5 text-[11px] text-slate-400">
          {{ t(section.noteKey) }}
        </p>
        <ul class="divide-y divide-slate-100">
          <li v-for="row in section.rows" :key="row.name" class="flex items-start gap-2 px-3 py-1">
            <button
              type="button"
              class="mono shrink-0 rounded px-1 text-[11px] text-sky-700 hover:bg-sky-50"
              :title="t('envVars.copyHint')"
              @click="copy(row.name)"
            >
              {{ row.name }}
            </button>
            <span class="min-w-0 flex-1 text-[11px] leading-relaxed break-words whitespace-pre-wrap text-slate-600">{{ row.desc }}</span>
          </li>
          <li v-if="!section.rows.length" class="px-3 py-1.5 text-[11px] text-slate-400">
            {{ t('common.empty') }}
          </li>
        </ul>
      </section>
    </div>

    <template #footer>
      <el-button size="small" @click="visible = false">{{ t('common.close') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { readFile } from '@/api/endpoints'
import { Manifest } from '@/utils/manifest'

const props = defineProps<{ project: string }>()
/** 用 v-model 控制显隐，和项目里其他弹窗一致。 */
const visible = defineModel<boolean>({ required: true })

const { t } = useI18n()

interface Row {
  name: string
  desc: string
}

interface Section {
  key: string
  labelKey: string
  noteKey?: string
  rows: Row[]
}

/**
 * 固定的 TRIM_* 系统变量。
 *
 * 清单与说明沿用 fnpackup 的「环境变量」页——这些名字是飞牛 install.sh 里
 * 约定的，写生命周期脚本时按这张表查。
 */
const STATIC_GROUPS: { key: string; labelKey: string; vars: string[] }[] = [
  { key: 'basic', labelKey: 'envVars.basic', vars: ['TRIM_APPNAME', 'TRIM_APPVER', 'TRIM_OLD_APPVER'] },
  {
    key: 'path',
    labelKey: 'envVars.pathInfo',
    vars: [
      'TRIM_APPDEST',
      'TRIM_PKGETC',
      'TRIM_PKGVAR',
      'TRIM_PKGTMP',
      'TRIM_PKGHOME',
      'TRIM_PKGMETA',
      'TRIM_APPDEST_VOL',
    ],
  },
  { key: 'network', labelKey: 'envVars.network', vars: ['TRIM_SERVICE_PORT'] },
  {
    key: 'user',
    labelKey: 'envVars.userPermission',
    vars: [
      'TRIM_USERNAME',
      'TRIM_GROUPNAME',
      'TRIM_UID',
      'TRIM_GID',
      'TRIM_RUN_USERNAME',
      'TRIM_RUN_GROUPNAME',
      'TRIM_RUN_UID',
      'TRIM_RUN_GID',
    ],
  },
  { key: 'share', labelKey: 'envVars.dataShare', vars: ['TRIM_DATA_SHARE_PATHS'] },
  {
    key: 'temp',
    labelKey: 'envVars.tempLog',
    vars: [
      'TRIM_TEMP_LOGFILE',
      'TRIM_TEMP_UPGRADE_FOLDER',
      'TRIM_PKGINST_TEMP_DIR',
      'TRIM_TEMP_TPKFILE',
    ],
  },
  { key: 'cmd', labelKey: 'envVars.cmdScript', vars: ['TRIM_APP_STATUS'] },
  { key: 'auth', labelKey: 'envVars.authPath', vars: ['TRIM_DATA_ACCESSIBLE_PATHS'] },
  {
    key: 'version',
    labelKey: 'envVars.versionInfo',
    vars: [
      'TRIM_SYS_VERSION',
      'TRIM_SYS_VERSION_MAJOR',
      'TRIM_SYS_VERSION_MINOR',
      'TRIM_SYS_VERSION_BUILD',
    ],
  },
  {
    key: 'sys',
    labelKey: 'envVars.sysFeature',
    vars: ['TRIM_SYS_ARCH', 'TRIM_KERNEL_VERSION', 'TRIM_SYS_MACHINE_ID', 'TRIM_SYS_LANGUAGE'],
  },
]

const WIZARD_FILES: { file: string; labelKey: string }[] = [
  { file: 'install', labelKey: 'envVars.wizardInstall' },
  { file: 'uninstall', labelKey: 'envVars.wizardUninstall' },
  { file: 'upgrade', labelKey: 'envVars.wizardUpgrade' },
  { file: 'config', labelKey: 'envVars.wizardConfig' },
]

const loading = ref(false)
const manifestRows = ref<Row[]>([])
const wizardSections = ref<Section[]>([])

const sections = computed<Section[]>(() => [
  ...STATIC_GROUPS.map((group) => ({
    key: group.key,
    labelKey: group.labelKey,
    rows: group.vars.map((name) => ({
      name: `\${${name}}`,
      desc: t(`envVars.vars.${name}`),
    })),
  })),
  {
    key: 'manifest',
    labelKey: 'envVars.manifest',
    noteKey: 'envVars.manifestNote',
    rows: manifestRows.value,
  },
  ...wizardSections.value,
])

/** 读项目里的 manifest 与向导，得到「这个项目自己提供了哪些变量」。 */
async function load() {
  loading.value = true
  manifestRows.value = []
  wizardSections.value = []
  try {
    const manifest = await readFile(`${props.project}/manifest`).catch(() => null)
    if (manifest) {
      manifestRows.value = Manifest.parse(manifest.content)
        .entries.filter((e) => e.kind === 'kv' && e.key)
        .map((e) => ({ name: `\${${e.key}}`, desc: e.value || '—' }))
    }

    const list: Section[] = []
    for (const item of WIZARD_FILES) {
      const res = await readFile(`${props.project}/wizard/${item.file}`).catch(() => null)
      const rows = res?.content ? wizardRows(res.content) : []
      if (!rows.length) continue
      list.push({
        key: `wizard-${item.file}`,
        labelKey: item.labelKey,
        noteKey: 'envVars.wizardNote',
        rows,
      })
    }
    wizardSections.value = list
  } finally {
    loading.value = false
  }
}

/** 向导里填的值会按字段名变成变量（不带 TRIM_ 前缀）。 */
function wizardRows(content: string): Row[] {
  try {
    const steps = JSON.parse(content) as unknown
    if (!Array.isArray(steps)) return []
    const rows: Row[] = []
    for (const step of steps) {
      const items = (step as { items?: unknown }).items
      if (!Array.isArray(items)) continue
      for (const item of items as Record<string, unknown>[]) {
        const field = String(item.field ?? '')
        if (!field) continue
        const label = String(item.label ?? item.helpText ?? '')
        const title = String((step as { stepTitle?: unknown }).stepTitle ?? '')
        rows.push({ name: `\${${field}}`, desc: [title, label].filter(Boolean).join(' / ') })
      }
    }
    return rows
  } catch {
    // 向导文件不是合法 JSON：不显示这一组，编辑器里会有更明确的报错。
    return []
  }
}

async function copy(text: string) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      // 非安全上下文（http://ip 访问）下没有 clipboard API，退回旧办法。
      const el = document.createElement('textarea')
      el.value = text
      el.style.position = 'fixed'
      el.style.opacity = '0'
      document.body.appendChild(el)
      el.select()
      document.execCommand('copy')
      document.body.removeChild(el)
    }
    ElMessage.success(t('common.copied'))
  } catch {
    ElMessage.error(t('envVars.copyFailed'))
  }
}

// 每次打开都重读一遍：写脚本时刚改过 manifest / 向导，这里要跟着变。
// immediate 是为了「挂载时 visible 已经是 true」这种用法也能加载（只依赖变化
// 的写法在这种情况下会一直空着）。
watch(
  visible,
  (open) => {
    if (open) void load()
  },
  { immediate: true },
)

defineExpose({ load })
</script>
