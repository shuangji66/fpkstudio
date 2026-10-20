<template>
  <div v-if="tab" class="flex h-full min-h-0 flex-col bg-white dark:bg-slate-800">
    <!--
      工具栏分两组：左边「后退 + 文件名 + 改动状态」，右边所有按钮。
      窄屏（移动端）放不下时整组按钮换行，而不是从中间随便断——一组按钮被拆散比
      整体下移难认得多。

      左组那个 min-w 是让换行按「组」发生的：flex 换行看的是各项目的基准宽度，
      左组是 flex-1（基准 0）的话它永远"放得下"，右组就不会整组下去，而是被挤在
      同一行里各自折行。给左组一个下限，宽度不够时右组才会整块换行。
      右组的 ml-auto 管的是换行之后的落位：它单独占一行时靠右对齐（同一行时
      剩余空间都被左组的 flex-1 吃掉了，auto margin 分不到东西，等于没设）。
    -->
    <div class="flex shrink-0 flex-wrap items-center gap-x-2 gap-y-1 border-b border-slate-200 dark:border-slate-700 px-2 py-1">
      <div class="flex min-w-[12rem] flex-1 items-center gap-2">
        <!--
          移动端：文件是在子页面里打开的，后退就是关掉这个文件标签。只画一个左箭头
          ——这一行本来就窄，而且它旁边就是文件名，写「后退」两个字是多余的。
          （不加 tooltip：触屏上点一下就会弹出来挡着，见 AGENTS 里的记录。）
        -->
        <el-button
          v-if="back"
          text
          size="small"
          class="tree-tool"
          :icon="ArrowLeft"
          :aria-label="t('files.back')"
          @click="back()"
        />
        <el-icon class="shrink-0 text-slate-400 dark:text-slate-500"><Document /></el-icon>
        <span class="mono min-w-0 flex-1 truncate text-[11px] text-slate-500 dark:text-slate-400">{{ tab.path }}</span>
        <!-- 磁盘上还没有这个文件：说「未创建」比说「未保存」准确 -->
        <span v-if="tab.missing" class="shrink-0 text-[11px] text-emerald-700 dark:text-emerald-400">
          ● {{ t('editor.notCreated') }}
        </span>
        <span v-else-if="tab.dirty" class="shrink-0 text-[11px] text-amber-600 dark:text-amber-400">
          ● {{ t('editor.unsaved') }}
        </span>
      </div>

      <div class="ml-auto flex flex-wrap items-center gap-2">
        <!--
          表单 ⇄ 源码：一个按钮，字面写的是**当前**那个视图，点一下换过去。以前是两格的
          单选滑块，工具栏里就它一种别的控件形态，看着像是另一个工具栏。
        -->
        <el-button v-if="def.hasForm && tab.text" size="default" @click="toggleMode">
          {{ t(mode === 'form' ? 'editor.form' : 'editor.source') }}
        </el-button>

        <!--
          cmd/main 最多三份（有进程 / 无进程 / docker），用下拉让用户选；其余有模板的
          文件只有一份，仍是普通按钮。下拉的 trigger=click：和上面「覆盖其他向导」
          一样，鼠标点开、触屏也点得开。
        -->
        <el-dropdown
          v-if="templateOptions.length > 1"
          trigger="click"
          @command="insertTemplate"
        >
          <el-button size="default">
            {{ t('cmdEditor.template') }}<el-icon class="ml-1"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="option in templateOptions" :key="option.key" :command="option.key">
                {{ t(option.labelKey) }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button
          v-else-if="templateOptions.length === 1"
          size="default"
          @click="insertTemplate(templateOptions[0]!.key)"
        >
          {{ t('cmdEditor.template') }}
        </el-button>
        <el-button v-if="isLifecycle" size="default" :icon="MessageBox" @click="envOpen = true">
          {{ t('envVars.button') }}
        </el-button>

        <!-- 安装 / 更新 / 配置向导常常是同一套表单：把手上这份写进别的向导 -->
        <el-dropdown v-if="wizardTargets.length" trigger="click" @command="onOverwrite">
          <el-button size="default" :icon="CopyDocument">
            {{ t('wizard.overwrite') }}<el-icon class="ml-1"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="name in wizardTargets" :key="name" :command="name">
                {{ t(wizardNameKey(name)) }}
              </el-dropdown-item>
              <el-dropdown-item divided command="all">{{ t('wizard.overwriteAll') }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>

        <!-- 图片预览（含 svg 的预览视图）没有任何可存的内容，这两个按钮只会是灰的 -->
        <el-button v-if="!previewOnly" size="default" :disabled="!tab.dirty" @click="revert">
          {{ t('editor.revert') }}
        </el-button>
        <el-button
          v-if="!previewOnly"
          size="default"
          type="primary"
          :disabled="!tab.dirty"
          :loading="saving"
          @click="save"
        >
          {{ saving ? t('editor.saving') : t('editor.save') }}
        </el-button>
      </div>
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
      <p class="text-xs text-slate-500 dark:text-slate-400">
        {{ tab.tooLarge ? t('editor.tooLarge', { size: tab.sizeRaw }) : t('editor.binary') }}
      </p>
      <el-button size="default" @click="workspace.downloadAt(`${project}/${tab.path}`)">
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
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, ArrowLeft, CopyDocument, Document, MessageBox } from '@element-plus/icons-vue'
import SourceEditor from './editors/SourceEditor.vue'
import EnvVarsDialog from './EnvVarsDialog.vue'
import { resolveEditor } from './registry'
import { handoffText } from '@/utils/handoff'
import { i18nTemplateFor } from '@/utils/i18nFiles'
import { mainTemplateOptions } from '@/utils/lifecycle'
import type { EditorApi, EditorContent } from './useEditor'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  project: string
  path: string
  /** 移动端传进来：在工具栏最左边渲染一个后退按钮（等价于关掉这个文件标签）。 */
  back?: () => void
}>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const inner = ref<(EditorApi & { getText?: () => string }) | null>(null)
const mode = ref<'form' | 'source'>('form')
const saving = ref(false)

/**
 * 表单 ⇄ 源码。按钮上写的是当前视图，点一下换到另一个——切换时该交哪份文本由
 * `watch(mode)`（utils/handoff.ts 的规则）处理，这里只改那一个状态。
 */
function toggleMode() {
  mode.value = mode.value === 'form' ? 'source' : 'form'
}

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

/**
 * 这个宿主实例对应的文件标签对象（按引用记住，不是按路径查）。
 *
 * 卸载时要把编辑器里没保存的内容交回 store，而组件是按 `file.path` 做 key 的：
 * 重命名会就地改掉标签的 path，卸载旧实例时 `props.path` 还是旧名字，按路径查会
 * 查不到——那份内容就交不回去，表现是「改个文件名，改到一半的内容没了」。
 * 标签对象本身在改名时是原地改写的，所以握着引用就一直是同一个。
 */
const tabRef = workspace.state(props.project)?.files.find((f) => f.path === props.path) ?? null

const def = computed(() => resolveEditor(props.path))

/**
 * 当前看的是「只能预览、没法编辑」的那一面。
 *
 * 图片（png / jpg…）没有表单，svg 的「表单」视图其实就是这张图——两种情况下
 * `getContent()` 都交不出可写的内容，还原 / 保存只会在那里灰着占位置，藏掉。
 * svg 切到「源码」之后照常可以改、可以存。
 */
const previewOnly = computed(() => def.value.previews === true && mode.value === 'form')

/**
 * 真正加载进编辑器的文本。
 *
 * handoff 里放的是"切换视图那一刻的实时内容"，两种视图都要用它——不能只在源码
 * 视图用：源码改完切回表单时，如果这里退回磁盘上的 tab.origin，未保存的修改就
 * 被丢掉了。
 *
 * draft 是上一次卸载这个编辑器时存下的内容：切换桌面 / 移动布局会把编辑器宿主
 * 整个换掉，没有它的话重新挂载会退回磁盘上那一份，未保存的编辑就这么没了。
 * 两者都为空（刚打开 / 保存后 / 还原后）才回落到磁盘内容。
 */
const loadText = computed(() => handoff.value || tab.value?.draft || tab.value?.origin || '')

const activeComponent = computed(() => {
  const d = def.value
  // 单视图编辑器（cmd、图片）就是它自己；表单类编辑器按 Form / Source 切。
  if (!d.hasForm) return d.component
  return mode.value === 'form' ? d.component : SourceEditor
})

/**
 * 能不能渲染出内容：文本文件可以，图片这类标了 previews 的也可以。
 *
 * 太大而没把内容带回来的文件（`tooLarge`）不算——后端只回了元信息，交给编辑器只会
 * 得到一份空文本，用户还以为文件是空的。
 */
const showEditor = computed(
  () =>
    !tab.value?.tooLarge && (Boolean(tab.value?.text) || def.value.previews === true),
)

// 卸载前把手里没保存的内容交回 store（见 loadText 的说明）。父组件的
// beforeUnmount 先于子组件卸载执行，所以这时候 inner 还是活的。
//
// 按 tabRef 当前的 path 交（而不是 props.path）：重命名会让这两者不一样，用旧名字
// 查不到标签，草稿就丢了。
onBeforeUnmount(() => {
  try {
    const content = inner.value?.getContent?.()?.content
    if (typeof content === 'string' && tabRef) {
      workspace.setDraft(props.project, tabRef.path, content)
    }
  } catch {
    // 编辑器没就绪就算了：宁可丢这一份草稿，也不要让卸载过程抛错
  }
})

watch(mode, async (next, previous) => {
  const editor = inner.value
  // 规则本身在 utils/handoff.ts（纯函数、有断言），这里只负责把两边的值递进去。
  //
  // `editorText` 必须在覆盖 handoff 之前从 loadText 取：它就是当前这个编辑器挂载时
  // 拿到的那份文本。表单模型不可信（解析失败降级成空模型）时交回的是它，而不是那份
  // 空模型——否则用户刚写的文本会被顶掉，而且空配置往往还「合法」，一保存就把文件
  // 清空（见 utils/handoff.ts 的文件头）。
  handoff.value = handoffText({
    from: previous,
    to: next,
    formDirty: editor?.isDirty?.() ?? false,
    formBlocked: editor?.saveBlocked?.() ?? '',
    formText: editor?.getContent?.()?.content ?? '',
    editorText: previous === 'source' ? (editor?.getText?.() ?? loadText.value) : loadText.value,
    diskText: tab.value?.origin ?? '',
  })
  await nextTick()
  onDirty(inner.value?.isDirty?.() ?? false)
})

function onDirty(value: boolean) {
  workspace.setDirty(props.project, props.path, value)
}

interface TemplateOption {
  key: string
  labelKey: string
  text: string
}

/**
 * 当前项目是不是 docker 应用：`cmd/main` 的下拉里要不要多给一份 docker 模板。
 *
 * 取 `workspace` 的状态而不是项目列表里的 `ProjectInfo`：这份布尔值由 `ensure()` 里的
 * `reloadShape()` 用 `/api/projects/exists` 填好（后端按 `app/docker/docker-compose.yaml`
 * 是否存在判定），而 `ProjectTab` 是 `await workspace.ensure()` 之后才渲染的，所以打开
 * `cmd/main` 时它一定已经就位；项目列表反而可能还没拉回来（直接开项目链接时）。
 */
const dockerProject = computed(() => workspace.state(props.project)?.docker === true)

const MAIN_LABEL: Record<string, string> = {
  process: 'cmdEditor.templateProcess',
  'no-process': 'cmdEditor.templateNoProcess',
  docker: 'cmdEditor.templateDocker',
}

/**
 * 「插入模板」的可选项，空数组表示这份文件没有通用模板、按钮不出现。
 *
 * - `cmd/main`：有进程 / 无进程（docker 项目多一份「应用中心托管」），用下拉选。
 * - `cmd/uninstall_callback`：fnOS 卸载时按向导里收集的 `$wizard_data_action` 决定
 *   删不删数据，这份骨架就是那个标准写法。
 * - 其余 `cmd/*`：**没有模板**（按钮不出现）。只有 main / uninstall_callback 有骨架。
 * - `wizard/uninstall`：和卸载回调配套的向导骨架（数据保留 / 删除二选一）。
 * - `i18n/en-US`、`i18n/zh-CN`：官方 section / key 骨架、值留空。
 *
 * 模板文本都在 `utils/lifecycle.ts` / `utils/i18nFiles.ts`（纯文本，官方脚手架原样，
 * 别在这里改写法）。写成 computed 而不是常量表：模板常量在下面，常量表会在求值时就踩到 TDZ。
 */
const templateOptions = computed<TemplateOption[]>(() => {
  const path = props.path
  if (path === 'cmd/main') {
    return mainTemplateOptions(dockerProject.value).map((option) => ({
      key: option.key,
      labelKey: MAIN_LABEL[option.key] ?? 'cmdEditor.template',
      text: option.text,
    }))
  }
  if (path === 'cmd/uninstall_callback') {
    return [{ key: 'uninstall', labelKey: 'cmdEditor.template', text: UNINSTALL_SCRIPT }]
  }
  if (path === 'wizard/uninstall') {
    return [{ key: 'uninstall-wizard', labelKey: 'cmdEditor.template', text: UNINSTALL_WIZARD }]
  }
  const locale = i18nTemplateFor(path)
  if (locale) return [{ key: 'locale', labelKey: 'cmdEditor.template', text: locale }]
  return []
})

/**
 * 生命周期脚本（cmd/ 下的安装、卸载、升级、配置、启停脚本）。
 *
 * 只有这类脚本需要查环境变量——TRIM_* 是安装器在跑脚本时注入的，向导里收集的
 * 值也在同一批变量里；manifest / 向导自己的编辑页不需要这个按钮。
 */
const isLifecycle = computed(() => props.path.startsWith('cmd/'))

/**
 * 安装 / 更新 / 配置三份向导往往就是同一套表单，所以给一个「覆盖其他向导」的入口：
 * 把手上这份的内容写进**另外的向导**（卸载向导的表单通常不一样，不参与）。
 *
 * 覆盖哪一份由用户在菜单里挑，或者选「全部覆盖」一次写两份——一次写两份是破坏性的，
 * 不该是唯一的选项。覆盖完顺手把目标文件打开：写磁盘的动作，用户总得能立刻看到
 * 写成了什么。
 *
 * 和以前的「复制其他向导」方向相反：那时是拉别人的内容进来、只改内存，得自己再
 * 打开目标文件保存一次。
 */
const WIZARD_GROUP = ['install', 'upgrade', 'config']

const currentWizard = computed(() => {
  const match = /^wizard\/([^/]+)$/.exec(props.path)
  return match && WIZARD_GROUP.includes(match[1]!) ? match[1]! : ''
})

/** 会被覆盖的另外两份向导。 */
const wizardTargets = computed(() =>
  currentWizard.value ? WIZARD_GROUP.filter((name) => name !== currentWizard.value) : [],
)

/** 向导文件的显示名复用快捷设置里的那套文案。 */
function wizardNameKey(name: string): string {
  return `shortcuts.wizard${name.charAt(0).toUpperCase()}${name.slice(1)}`
}

function onOverwrite(command: string) {
  const targets = command === 'all' ? wizardTargets.value : [command]
  void overwriteWizards(targets)
}

async function overwriteWizards(targets: string[]) {
  if (!targets.length) return
  const names = targets.map((name) => t(wizardNameKey(name))).join(' / ')

  try {
    await ElMessageBox.confirm(t('wizard.overwriteConfirm', { names }), t('common.tip'), {
      type: 'warning',
      confirmButtonText: t('wizard.overwriteOk'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }

  // 和「保存」同一条规矩：模型不可信（解析失败降级成空模型）时不能把它写进别的文件
  // ——那会把目标向导整份覆盖掉，而且不可逆。以前这里只问了 getContent，没问
  // saveBlocked，表现是「当前向导解析失败，点一下『覆盖其他向导』就把另两份清成 []」。
  const blocked = inner.value?.saveBlocked?.()
  if (blocked) {
    ElMessage.error(blocked)
    return
  }
  const payload = inner.value?.getContent?.() ?? null
  if (!payload) {
    ElMessage.error(t('editor.notReady'))
    return
  }

  try {
    for (const target of targets) {
      const rel = `wizard/${target}`
      // saveFile 会连父目录一起建出来，所以目标文件不存在时这里就是「创建」。
      await workspace.saveFile(props.project, rel, payload.content)
      // 打开（或切到）这份文件；已经开着的那份要重新读一次，否则标签里还是旧内容。
      await workspace.openFile(props.project, `${props.project}/${rel}`)
      await workspace.reloadFile(props.project, rel)
    }
    await workspace.refreshDir(props.project, `${props.project}/wizard`).catch(() => undefined)
    ElMessage.success(t('wizard.overwritten', { names }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

const UNINSTALL_SCRIPT = `#!/bin/bash

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

/**
 * 卸载向导的骨架：先让用户选数据怎么处理，再把选择交给上面那个卸载回调脚本
 * （`$wizard_data_action`）。两份是一套的，所以放在一起。
 */
const UNINSTALL_WIZARD = `[
  {
    "stepTitle": "确认卸载",
    "items": [
      {
        "type": "tips",
        "field": "wizard_default",
        "label": "示例项",
        "rules": [],
        "options": [],
        "helpText": "您即将卸载此应用。请选择如何处理应用数据："
      },
      {
        "type": "radio",
        "field": "wizard_data_action",
        "label": "数据保留选项",
        "rules": [
          {
            "required": true,
            "message": "请选择数据保留选项"
          }
        ],
        "options": [
          {
            "label": "保留数据（推荐）- 将来重新安装时可恢复",
            "value": "keep"
          },
          {
            "label": "删除所有数据 - 此操作不可恢复！",
            "value": "delete"
          }
        ],
        "initValue": "keep"
      },
      {
        "type": "tips",
        "field": "wizard_default",
        "label": "示例项",
        "rules": [],
        "options": [],
        "helpText": "<strong>警告：</strong> 选择删除数据后，所有应用数据将永久丢失，无法恢复。"
      }
    ]
  }
]
`

async function insertTemplate(key: string) {
  const option = templateOptions.value.find((item) => item.key === key) ?? templateOptions.value[0]
  if (!option) return
  const editor = inner.value
  if (!editor?.setContent) return
  editor.setContent(option.text)
  await nextTick()
  onDirty(editor.isDirty?.() ?? false)
}

async function save() {
  // 有些编辑器的内容现在就是不可信的（例如向导文件解析失败，表单里是空的）。
  // 这种时候必须拦住：保存下去等于把用户的文件写成一份空向导。
  const blocked = inner.value?.saveBlocked?.()
  if (blocked) {
    ElMessage.error(blocked)
    return
  }
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
    // 不切回表单视图：在源码里改完还想接着改是常态，视图跳走只会让人再点回来。
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
