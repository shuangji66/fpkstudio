<template>
  <div v-if="tab" class="flex h-full min-h-0 flex-col bg-white dark:bg-slate-800">
    <div class="flex shrink-0 items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-2 py-1">
      <el-icon class="shrink-0 text-slate-400 dark:text-slate-500"><Document /></el-icon>
      <span class="mono min-w-0 flex-1 truncate text-[11px] text-slate-500 dark:text-slate-400">{{ tab.path }}</span>
      <!-- 磁盘上还没有这个文件：说「未创建」比说「未保存」准确 -->
      <span v-if="tab.missing" class="shrink-0 text-[11px] text-emerald-700 dark:text-emerald-400">
        ● {{ t('editor.notCreated') }}
      </span>
      <span v-else-if="tab.dirty" class="shrink-0 text-[11px] text-amber-600 dark:text-amber-400">
        ● {{ t('editor.unsaved') }}
      </span>

      <el-radio-group v-if="def.hasForm && tab.text" v-model="mode" size="small">
        <el-radio-button value="form">{{ t('editor.form') }}</el-radio-button>
        <el-radio-button value="source">{{ t('editor.source') }}</el-radio-button>
      </el-radio-group>

      <el-button v-if="canInsertTemplate" size="small" @click="insertTemplate">
        {{ t('cmdEditor.template') }}
      </el-button>
      <el-button v-if="isLifecycle" size="small" :icon="MessageBox" @click="envOpen = true">
        {{ t('envVars.button') }}
      </el-button>

      <!-- 安装 / 更新 / 配置向导常常是同一套表单，允许互相整体覆盖 -->
      <el-dropdown v-if="wizardSources.length" trigger="click" @command="copyWizard">
        <el-button size="small" :icon="CopyDocument">
          {{ t('wizard.copyFrom') }}<el-icon class="ml-1"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="source in wizardSources" :key="source" :command="source">
              {{ t(wizardNameKey(source)) }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <el-button size="small" :disabled="!tab.dirty" @click="revert">
        {{ t('editor.revert') }}
      </el-button>
      <el-button size="small" type="primary" :disabled="!tab.dirty" :loading="saving" @click="save">
        {{ saving ? t('editor.saving') : t('editor.save') }}
      </el-button>
    </div>

    <div
      v-if="tab.error"
      class="shrink-0 border-b border-amber-200 dark:border-amber-900 bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-xs text-amber-800 dark:text-amber-200"
    >
      {{ tab.error }}
    </div>

    <div v-if="tab.loading" class="flex flex-1 items-center justify-center text-xs text-slate-400 dark:text-slate-500">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="!showEditor" class="flex flex-1 flex-col items-center justify-center gap-3">
      <p class="text-xs text-slate-500 dark:text-slate-400">{{ t('editor.binary') }}</p>
      <el-button size="small" @click="workspace.downloadAt(`${project}/${tab.path}`)">
        {{ t('common.download') }}
      </el-button>
    </div>

    <div v-else class="min-h-0 flex-1">
      <component
        :is="activeComponent"
        ref="inner"
        :key="`${tab.path}:${mode}`"
        :path="tab.path"
        :origin="loadText"
        :original="tab.origin"
        :project="project"
        @dirty="onDirty"
      />
    </div>

    <EnvVarsDialog v-if="isLifecycle" v-model="envOpen" :project="project" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, CopyDocument, Document, MessageBox } from '@element-plus/icons-vue'
import SourceEditor from './editors/SourceEditor.vue'
import EnvVarsDialog from './EnvVarsDialog.vue'
import { resolveEditor } from './registry'
import type { EditorApi, EditorContent } from './useEditor'
import { readFile } from '@/api/endpoints'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{ project: string; path: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const inner = ref<(EditorApi & { getText?: () => string }) | null>(null)
const mode = ref<'form' | 'source'>('form')
const saving = ref(false)

/** 环境变量弹窗的显隐（只有生命周期脚本有这个按钮）。 */
const envOpen = ref(false)

/** 进入源码视图时，用它接住表单视图当前的实时内容。 */
const handoff = ref('')

/**
 * 从 store 按路径取当前标签，而不是把对象当 prop 传进来。
 *
 * 传对象时组件持有的是某个时刻的引用；一旦两边指向不同实例，就会出现
 * 「数据已经写进去了但界面不动」。按路径查，永远是当前那一个。
 */
const tab = computed(() => workspace.state(props.project)?.files.find((f) => f.path === props.path))

const def = computed(() => resolveEditor(props.path))

/**
 * 真正加载进编辑器的文本。
 *
 * handoff 里放的是"切换视图那一刻的实时内容"，两种视图都要用它——不能只在源码
 * 视图用：源码改完切回表单时，如果这里退回磁盘上的 tab.origin，未保存的修改就
 * 被丢掉了。只有 handoff 为空（刚打开 / 保存后 / 还原后）才回落到磁盘内容。
 */
const loadText = computed(() => handoff.value || tab.value?.origin || '')

const activeComponent = computed(() => {
  const d = def.value
  // 单视图编辑器（cmd、图片）就是它自己；表单类编辑器按 Form / Source 切。
  if (!d.hasForm) return d.component
  return mode.value === 'form' ? d.component : SourceEditor
})

/** 能不能渲染出内容：文本文件可以，图片这类标了 previews 的也可以。 */
const showEditor = computed(() => Boolean(tab.value?.text) || def.value.previews === true)

watch(
  () => props.path,
  () => {
    mode.value = 'form'
    handoff.value = ''
  },
)

watch(mode, async (next, previous) => {
  if (previous === 'form' && next === 'source') {
    // 表单 → 源码：把序列化结果带过去，避免用户刚才的编辑消失
    handoff.value = inner.value?.getContent?.()?.content ?? tab.value?.origin ?? ''
  } else if (previous === 'source' && next === 'form') {
    // 源码 → 表单：把源码内容作为解析输入
    handoff.value = inner.value?.getText?.() ?? tab.value?.origin ?? ''
  }
  await nextTick()
  onDirty(inner.value?.isDirty?.() ?? false)
})

function onDirty(value: boolean) {
  workspace.setDirty(props.project, props.path, value)
}

/**
 * 只有卸载回调脚本给「插入模板」。
 *
 * fnOS 卸载时会调用它，并按向导里收集的 $wizard_data_action 决定删不删数据；
 * 这份骨架就是那个标准写法。其他 cmd 脚本（main/start/stop…）各写各的，
 * 没有通用模板可插，所以按钮不出现。
 */
const canInsertTemplate = computed(() => props.path === 'cmd/uninstall_callback')

/**
 * 生命周期脚本（cmd/ 下的安装、卸载、升级、配置、启停脚本）。
 *
 * 只有这类脚本需要查环境变量——TRIM_* 是安装器在跑脚本时注入的，向导里收集的
 * 值也在同一批变量里；manifest / 向导自己的编辑页不需要这个按钮。
 */
const isLifecycle = computed(() => props.path.startsWith('cmd/'))

/**
 * 可以整体复制过来的其他向导。
 *
 * 安装 / 更新 / 配置三份向导往往就是同一套表单（fnpackup 里只能开着源码手动复制
 * 粘贴），所以给这三份之间做一个「整份覆盖」的入口；卸载向导的表单通常不一样，
 * 不参与互拷。
 */
const WIZARD_COPYABLE = ['install', 'upgrade', 'config']

const currentWizard = computed(() => {
  const match = /^wizard\/([^/]+)$/.exec(props.path)
  return match && WIZARD_COPYABLE.includes(match[1]!) ? match[1]! : ''
})

const wizardSources = computed(() =>
  currentWizard.value ? WIZARD_COPYABLE.filter((name) => name !== currentWizard.value) : [],
)

/** 向导文件的显示名复用快捷设置里的那套文案。 */
function wizardNameKey(name: string): string {
  return `shortcuts.wizard${name.charAt(0).toUpperCase()}${name.slice(1)}`
}

/** 把另一份向导的内容整份覆盖到当前编辑器（只改内存，保存后才落盘）。 */
async function copyWizard(source: string) {
  const label = t(wizardNameKey(source))
  try {
    await ElMessageBox.confirm(t('wizard.copyConfirm', { name: label }), t('common.tip'), {
      type: 'warning',
      confirmButtonText: t('wizard.copyConfirmOk'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }

  const editor = inner.value
  if (!editor?.setContent) {
    ElMessage.error(t('editor.notReady'))
    return
  }

  try {
    const res = await readFile(`${props.project}/wizard/${source}`)
    if (!res.text) {
      ElMessage.error(t('wizard.copyNotText', { name: label }))
      return
    }
    editor.setContent(res.content)
    await nextTick()
    onDirty(editor.isDirty?.() ?? false)
    ElMessage.success(t('wizard.copied', { name: label }))
  } catch (e) {
    ElMessage.error(t('wizard.copyFailed', { name: label, error: e instanceof Error ? e.message : String(e) }))
  }
}

const UNINSTALL_TEMPLATE = `#!/bin/bash

# 获取用户在向导中的选择
WIZARD_DATA_ACTION="$wizard_data_action"

echo "卸载时，用户选择数据保留选项: $WIZARD_DATA_ACTION"

# 根据用户选择执行相应逻辑
if [ "$WIZARD_DATA_ACTION" = "delete" ]; then
    echo "开始删除数据目录：$TRIM_PKGVAR "
    rm -rf \${TRIM_PKGVAR}
else
    echo "保留数据不动"
fi

exit 0
`

async function insertTemplate() {
  const editor = inner.value
  if (!editor?.setContent) return
  editor.setContent(UNINSTALL_TEMPLATE)
  await nextTick()
  onDirty(editor.isDirty?.() ?? false)
}

async function save() {
  const payload: EditorContent | null = inner.value?.getContent?.() ?? null
  if (!payload) {
    // 取不到内容说明编辑器没有 defineExpose(api)。以前这里直接 return，
    // 表现是「点保存什么都不发生」，连个报错都没有。
    ElMessage.error(t('editor.notReady'))
    return
  }
  saving.value = true
  try {
    await workspace.saveFile(props.project, props.path, payload.content)
    handoff.value = ''
    onDirty(false)
    if (mode.value === 'source' && def.value.hasForm) mode.value = 'form'
    ElMessage.success(t('editor.saved', { path: props.path }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    saving.value = false
  }
}

async function revert() {
  handoff.value = ''
  await workspace.reloadFile(props.project, props.path)
  await nextTick()
  onDirty(false)
  ElMessage.info(t('editor.revert'))
}
</script>
