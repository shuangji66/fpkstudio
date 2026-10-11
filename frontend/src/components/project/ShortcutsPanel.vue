<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-4">
    <div class="mx-auto max-w-5xl">
      <p class="rounded-md bg-slate-50 px-3 py-2 text-xs leading-relaxed text-slate-600">
        {{ t('shortcuts.hint') }}
      </p>

      <!--
        流式瀑布：卡片按列依次落下，而不是被网格一行一行地拉齐。
        多列布局（columns）而不是 grid，正是为了让长短不一的几组各自紧贴上一张卡片，
        组与组之间不留「高度差」造成的空洞；窄屏只有一列。
      -->
      <div class="mt-3 columns-1 gap-3 lg:columns-2">
        <section
          v-for="group in groups"
          :key="group.key"
          class="mb-3 break-inside-avoid rounded-lg border border-slate-200"
        >
          <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-2">
            <span class="text-xs font-medium text-slate-700">{{ t(group.labelKey) }}</span>
            <span class="flex-1" />
            <span class="text-[11px] text-slate-400">{{ t(group.hintKey) }}</span>
          </header>

          <ul class="divide-y divide-slate-100">
            <li v-for="item in group.items" :key="item.path">
              <button
                type="button"
                class="flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors hover:bg-sky-50/70"
                :title="`${props.project}/${item.path}`"
                @click="open(item.path)"
              >
                <span class="w-36 shrink-0 truncate text-xs text-slate-700">
                  {{ t(item.labelKey) }}
                </span>
                <span class="mono min-w-0 flex-1 truncate text-[11px] text-slate-400">
                  {{ item.path }}
                </span>
                <el-tag
                  v-if="opened(item.path)"
                  size="small"
                  effect="plain"
                  :type="dirty(item.path) ? 'warning' : 'info'"
                >
                  {{ dirty(item.path) ? t('editor.unsaved') : t('shortcuts.opened') }}
                </el-tag>
                <el-icon class="shrink-0 text-[12px] text-slate-300"><ArrowRight /></el-icon>
              </button>
            </li>
          </ul>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const state = computed(() => workspace.state(props.project))

interface Shortcut {
  path: string
  labelKey: string
}

interface Group {
  key: string
  labelKey: string
  hintKey: string
  /** 只在带桌面入口的项目里出现。 */
  needsUi?: boolean
  /** 只在 docker 应用里出现。 */
  needsDocker?: boolean
  items: Shortcut[]
}

/**
 * 快捷设置的可跳转项。
 *
 * 与 fnpackup 的「快速编辑」对齐，但按 fpkstudio 的形态做了两处调整：
 *  - 图标不在这里（图标已经有统一的处理标签页），环境变量也不在这里
 *    （它跟着生命周期脚本的编辑页走，按钮在编辑器工具栏上）；
 *  - 「有桌面入口 / 是 docker 应用」直接决定哪些项存在。
 */
const ALL_GROUPS: Group[] = [
  {
    key: 'base',
    labelKey: 'shortcuts.groupBase',
    hintKey: 'shortcuts.groupBaseHint',
    items: [{ path: 'manifest', labelKey: 'shortcuts.manifest' }],
  },
  {
    key: 'security',
    labelKey: 'shortcuts.groupSecurity',
    hintKey: 'shortcuts.groupSecurityHint',
    items: [
      { path: 'config/privilege', labelKey: 'shortcuts.privilege' },
      { path: 'config/resource', labelKey: 'shortcuts.resource' },
    ],
  },
  {
    key: 'entry',
    labelKey: 'shortcuts.groupEntry',
    hintKey: 'shortcuts.groupEntryHint',
    needsUi: true,
    items: [{ path: 'app/ui/config', labelKey: 'shortcuts.uiConfig' }],
  },
  {
    key: 'wizard',
    labelKey: 'shortcuts.groupWizard',
    hintKey: 'shortcuts.groupWizardHint',
    items: [
      { path: 'wizard/install', labelKey: 'shortcuts.wizardInstall' },
      { path: 'wizard/uninstall', labelKey: 'shortcuts.wizardUninstall' },
      { path: 'wizard/upgrade', labelKey: 'shortcuts.wizardUpgrade' },
      { path: 'wizard/config', labelKey: 'shortcuts.wizardConfig' },
    ],
  },
  {
    key: 'cmd',
    labelKey: 'shortcuts.groupCmd',
    hintKey: 'shortcuts.groupCmdHint',
    items: [
      { path: 'cmd/main', labelKey: 'shortcuts.cmdMain' },
      { path: 'cmd/install_init', labelKey: 'shortcuts.cmdInstallInit' },
      { path: 'cmd/install_callback', labelKey: 'shortcuts.cmdInstallCallback' },
      { path: 'cmd/uninstall_init', labelKey: 'shortcuts.cmdUninstallInit' },
      { path: 'cmd/uninstall_callback', labelKey: 'shortcuts.cmdUninstallCallback' },
      { path: 'cmd/upgrade_init', labelKey: 'shortcuts.cmdUpgradeInit' },
      { path: 'cmd/upgrade_callback', labelKey: 'shortcuts.cmdUpgradeCallback' },
      { path: 'cmd/config_init', labelKey: 'shortcuts.cmdConfigInit' },
      { path: 'cmd/config_callback', labelKey: 'shortcuts.cmdConfigCallback' },
    ],
  },
  {
    key: 'i18n',
    labelKey: 'shortcuts.groupI18n',
    hintKey: 'shortcuts.groupI18nHint',
    items: [
      { path: 'i18n/zh-CN', labelKey: 'shortcuts.i18nZh' },
      { path: 'i18n/en-US', labelKey: 'shortcuts.i18nEn' },
    ],
  },
  {
    key: 'docker',
    labelKey: 'shortcuts.groupDocker',
    hintKey: 'shortcuts.groupDockerHint',
    needsDocker: true,
    items: [{ path: 'app/docker/docker-compose.yaml', labelKey: 'shortcuts.dockerCompose' }],
  },
]

const groups = computed(() =>
  ALL_GROUPS.filter(
    (group) =>
      (!group.needsUi || state.value.ui) && (!group.needsDocker || state.value.docker),
  ),
)

function find(path: string) {
  return state.value.files.find((f) => f.path === path)
}

/** 已经打开了就在子标签里切过去，没打开就打开——打开文件由 store 统一处理。 */
function open(path: string) {
  void workspace.openFile(props.project, `${props.project}/${path}`)
}

const opened = (path: string) => Boolean(find(path))
const dirty = (path: string) => find(path)?.dirty === true
</script>
