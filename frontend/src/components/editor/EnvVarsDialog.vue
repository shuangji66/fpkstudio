<template>
  <el-dialog
    v-model="visible"
    :title="t('envVars.title')"
    class="app-dialog"
    modal-class="app-modal"
    :width="layout.isMobile ? '95%' : '46rem'"
    align-center
    lock-scroll
    append-to-body
    :close-on-click-modal="false"
  >
    <p class="mb-2 rounded-md bg-slate-50 dark:bg-slate-900 px-3 py-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
      {{ t('envVars.hint') }}
      <code class="mono rounded bg-white dark:bg-slate-800 px-1 text-slate-700 dark:text-slate-200">${TRIM_APPNAME}</code>
    </p>

    <div v-if="loading" class="py-6 text-center text-sm text-slate-400 dark:text-slate-500">
      {{ t('common.loading') }}
    </div>

    <div v-else class="space-y-3 pr-1">
      <section
        v-for="section in sections"
        :key="section.key"
        class="rounded-lg border border-slate-200 dark:border-slate-700"
      >
        <header class="flex items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-3 py-1.5">
          <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t(section.labelKey) }}</span>
          <span class="flex-1" />
          <span class="text-sm text-slate-400 dark:text-slate-500">{{ section.rows.length }}</span>
        </header>
        <p v-if="section.noteKey" class="px-3 pt-1.5 text-sm text-slate-400 dark:text-slate-500">
          {{ t(section.noteKey) }}
        </p>
        <ul class="divide-y divide-slate-100 dark:divide-slate-700">
          <!--
            flex-wrap：一行放不下就换行，不要让说明被挤成 0 宽。
            常用运行时那几条是一整行 export PATH=…（手机宽度下比弹窗还宽），
            复制按钮得能自己折行（min-w-0 break-all，shrink-0 会顶住不缩），
            这时说明会整块落到下一行，而不是被压成一条竖着排字的 0 宽列。
          -->
          <li v-for="row in section.rows" :key="row.name" class="flex flex-wrap items-start gap-2 px-3 py-1">
            <button
              type="button"
              class="mono min-w-0 rounded px-1 text-left text-sm break-all text-sky-700 dark:text-sky-300 hover:bg-sky-50 dark:hover:bg-sky-950/40"
              :title="t('envVars.copyHint')"
              @click="copy(row.name)"
            >
              {{ row.name }}
            </button>
            <span class="min-w-0 flex-1 text-sm leading-relaxed break-words whitespace-pre-wrap text-slate-600 dark:text-slate-300">{{ row.desc }}</span>
          </li>
          <li v-if="!section.rows.length" class="px-3 py-1.5 text-sm text-slate-400 dark:text-slate-500">
            {{ t('common.empty') }}
          </li>
        </ul>
      </section>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { readFile } from '@/api/endpoints'
import { useLayoutStore } from '@/stores/layout'
import { Manifest } from '@/utils/manifest'

const props = defineProps<{ project: string }>()
/** 用 v-model 控制显隐，和项目里其他弹窗一致。 */
const visible = defineModel<boolean>({ required: true })

const { t } = useI18n()
const layout = useLayoutStore()

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
 *
 * `vars` 里的项会渲染成 `${<项>}`，所以除了裸变量名，也可以放一段**变量上的 shell
 * 展开**（比如取列表里的第一项）：点一下复制走的就是整段能直接粘进脚本的写法。
 * 这种项在语言包里的键就是它自己（`envVars.vars.<项>`）。
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
  {
    key: 'share',
    labelKey: 'envVars.dataShare',
    // 除了裸变量，这里再放一段 shell 展开：脚本里多数时候只要第一个共享路径
    vars: ['TRIM_DATA_SHARE_PATHS', 'TRIM_DATA_SHARE_PATHS%%:*'],
  },
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

/**
 * 常用运行时。
 *
 * 飞牛把各语言的运行时装在 /var/apps/ 下，脚本里跑 node / java / python 之前得先把
 * 它挂进 PATH。这些不是「变量」而是一整条命令，所以单独列一组、原样列出来点一下复制。
 *
 * 命令里的版本号是占位符（`<version>`）：复制走的是**模板**，用户按自己机器上的版本
 * 改一下就行，所以另一份「能直接用的实例」不作为可复制项列出来——只在说明里点一句
 * 例子（`example`）长什么样。
 */
const RUNTIME_ROWS: { name: string; example: string }[] = [
  {
    name: 'export PATH=/var/apps/nodejs_v<version>/target/bin:$PATH',
    example: 'nodejs_v24',
  },
  {
    name: 'export PATH=/var/apps/java-<version>-openjdk/target/bin:$PATH',
    example: 'java-21-openjdk',
  },
  {
    name: 'export PATH=/var/apps/python<version>/target/bin:$PATH',
    example: 'python312',
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
  // 向导里收集的字段排最上面：写生命周期脚本时要填的往往就是这些「用户填过什么」，
  // 系统变量与运行时反而更像查表。manifest 那组位置不动（仍在运行时之后）。
  ...wizardSections.value,
  ...STATIC_GROUPS.map((group) => ({
    key: group.key,
    labelKey: group.labelKey,
    rows: group.vars.map((name) => ({
      name: `\${${name}}`,
      desc: t(`envVars.vars.${name}`),
    })),
  })),
  {
    key: 'runtime',
    labelKey: 'envVars.runtime',
    rows: RUNTIME_ROWS.map((row) => ({
      name: row.name,
      desc: t('envVars.runtimeExample', { example: row.example }),
    })),
  },
  {
    key: 'manifest',
    labelKey: 'envVars.manifest',
    noteKey: 'envVars.manifestNote',
    rows: manifestRows.value,
  },
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

</script>
