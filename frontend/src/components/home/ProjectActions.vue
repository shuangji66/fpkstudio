<template>
  <!--
    操作区：文件夹 + 下拉。整块自己挡住点击 / 键盘事件，否则会连带触发卡片自己的
    「打开项目」——卡片整块可点。
  -->
  <div class="inline-flex items-center" @click.stop @keydown.stop>
    <el-button
      text
      size="default"
      :icon="FolderOpened"
      :disabled="!canOpenFolder"
      :title="canOpenFolder ? t('common.openFolder') : t('trim.error.hostUnavailable')"
      @click="openFolder"
    />
    <el-dropdown trigger="click" placement="bottom-end" @command="run">
      <el-button text size="default" :icon="MoreFilled" :title="t('home.more')" />
      <template #dropdown>
        <el-dropdown-menu>
          <!-- 置顶是纯界面动作（状态只在浏览器里），排在最前面 -->
          <el-dropdown-item command="pin" :icon="Top">
            {{ pinned ? t('home.unpin') : t('home.pin') }}
          </el-dropdown-item>
          <el-dropdown-item command="rename" :icon="EditPen">
            {{ t('common.rename') }}
          </el-dropdown-item>
          <el-dropdown-item command="duplicate" :icon="CopyDocument">
            {{ t('home.duplicate') }}
          </el-dropdown-item>
          <!-- 标签只写「导出」：括号里那句说明是菜单里唯一的长文案，放这里只会把菜单撑宽 -->
          <el-dropdown-item command="export" :icon="Download">
            {{ t('home.export') }}
          </el-dropdown-item>
          <!-- 删除是破坏性的，红色标出来（和文件管理器的右键菜单同一套） -->
          <el-dropdown-item
            command="delete"
            :icon="Delete"
            divided
            class="text-red-600 dark:text-red-400"
          >
            {{ t('common.delete') }}
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument, Delete, Download, EditPen, FolderOpened, MoreFilled, Top } from '@element-plus/icons-vue'
import { copyEntry, projectExists, readFile, renameEntry, writeFile } from '@/api/endpoints'
import { useProjectsStore } from '@/stores/projects'
import { useTabsStore } from '@/stores/tabs'
import { useWorkspaceStore } from '@/stores/workspace'
import { useDownload } from '@/components/project/useDownload'
import { isHostAvailable, openInFileManager } from '@/trim/sdk'
import { trimErrorMessage } from '@/trim/errors'
import { rewriteForDuplicate } from '@/utils/duplicate'
import type { ProjectInfo } from '@/api/types'

/**
 * 项目卡片右上角的操作区：文件夹按钮（在文件管理器里打开项目目录）+「···」菜单
 * （置顶 / 重命名 / 创建副本 / 导出 / 删除）。
 *
 * 三个动作整条流程都在这里走完（改目录 → 处理项目标签 → 播报 → 最后刷列表）。
 * 顺序不能反：`projects.refresh()` 会换掉列表，外层 v-for 的 key 跟着变，
 * 这张卡片会被卸载——刷新之后再往外 emit 事件、或者再做依赖组件状态的事就来不及了。
 * 所以刷列表永远是最后一步。
 *
 * 删除只有这一条路，并且一定要过一遍确认弹窗——项目目录是直接删掉的，没有回收站。
 * 重命名改的是**项目目录名**（应用标识在 manifest.appname 里，不在这里动）。
 *
 * 置顶（`pin`）只是界面偏好：状态在浏览器里（见 `stores/projects.ts` 的 pinned），
 * 不请求后端，所以它是唯一一个「点完什么都不用等」的动作——但**改名 / 删除时要把
 * 置顶表一起改**，否则会留下一条指向不存在项目的记录（见下面的 rename / remove）。
 */
const props = defineProps<{ item: ProjectInfo }>()

const { t, te } = useI18n()
const projects = useProjectsStore()
const tabs = useTabsStore()
const workspace = useWorkspaceStore()
const { download } = useDownload()

/** 这个项目当前是不是置顶状态：菜单项在「置顶 / 取消置顶」之间切。 */
const pinned = computed(() => projects.isPinned(props.item.name))

/**
 * 文件管理器是宿主能力：独立浏览器页面里没有它可开；路径也必须是后端给出来的
 * 绝对路径（项目根目录可以被换成授权目录，拼 projectsDir + name 会拼错）。
 */
const canOpenFolder = computed(() => isHostAvailable() && Boolean(props.item.absPath))

async function openFolder() {
  try {
    await openInFileManager(props.item.absPath)
  } catch (e) {
    ElMessage.error(trimErrorMessage(t, te, e))
  }
}

/** 目录名规则与 fnpack create 保持一致：字母数字开头，可含 . _ -。 */
const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/

/**
 * 应用标识（appname）的规则：3–32 位、字母数字开头结尾，中间可含 . _ -。
 *
 * 「创建副本」会把新项目名直接写进副本 manifest 的 `appname`，所以要按 appname 的规则
 * 校验——比目录名严：名字太短 / 太长 / 以 `.` 结尾都过不了 fnpack，而报错会等到打包那一步
 * 才出现（「appname ilegal」），用户完全不知道是自己起名字时就错了。
 */
const APPNAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{1,30}[A-Za-z0-9]$/

function run(command: string) {
  if (command === 'pin') projects.togglePin(props.item.name)
  else if (command === 'rename') void rename()
  else if (command === 'duplicate') void duplicate()
  else if (command === 'export') exportProject()
  else if (command === 'delete') void remove()
}

/**
 * 导出：把项目目录打成 zip 交给浏览器下载（后端对目录就是这么处理的，包内带顶层目录）。
 *
 * 走后端现压缩这条路，所以挂 `zip: true`：稍大的项目能压好几秒，得先告诉用户「在动」，
 * 否则看起来像没点中（见 `useDownload`）。
 */
function exportProject() {
  void download(props.item.name, { zip: true })
}

async function rename() {
  // 先记住旧目录名：下面刷新列表之后 props.item 就是新项目了
  const from = props.item.name
  const dirty = workspace.dirtyCount(from) > 0
  let next = ''
  try {
    const res = await ElMessageBox.prompt(
      t(dirty ? 'home.renamePromptDirty' : 'home.renamePrompt', { name: from }),
      t('common.rename'),
      {
        inputValue: from,
        inputPattern: NAME_RE,
        inputErrorMessage: t('home.renameInvalid'),
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel'),
      },
    )
    next = String(res.value ?? '').trim()
  } catch {
    return // 取消
  }
  if (!next || next === from) return

  try {
    await renameEntry(from, next)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
    return
  }
  // 置顶表里存的是项目名，改名之后要跟着换（否则那条置顶就指向一个不存在的项目）
  projects.renamePinned(from, next)

  // 目录名换了，原来那个项目标签指向的路径已经不存在：关掉，按新名字重开一个
  // （未保存的改动在上面提醒过）。新标签的标题先落成目录名，列表刷新后
  // AppShell 会按 appname 再改一次。
  if (tabs.tabs.some((tab) => tab.id === tabs.projectTabId(from))) {
    tabs.closeProject(from)
    tabs.openProject(next)
    void workspace.ensure(next)
  }
  ElMessage.success(t('home.renamed', { name: next }))

  await projects.refresh()
}

/**
 * 创建副本：把整个项目目录复制成一份新项目，再把里面的应用标识改成副本名。
 *
 * 只改两处（`utils/duplicate.ts` 里有注释和断言）：`manifest` 的 `appname` + 默认入口 id
 * 的前缀、`app/ui/config` 里所有入口 id 的前缀。**其他文件原样复制**——图标、脚本、
 * 向导里的字段名都跟应用标识无关，改了反而是错的。
 *
 * 目标重名的保护在后端（`Root.Copy` 对已存在的目标直接回 `ErrExists`，不覆盖），这里先查
 * 一次只是为了给一句更直白的提示：`projects.list` 可能是旧的，所以问的是后端。
 */
async function duplicate() {
  const from = props.item.name
  let next = ''
  try {
    const res = await ElMessageBox.prompt(t('home.duplicatePrompt', { name: from }), t('home.duplicate'), {
      // 刻意不给默认值：副本名得自己想，随手回车只会撞上「已经有一个叫 xxx 的项目」
      inputPattern: APPNAME_RE,
      inputErrorMessage: t('home.duplicateInvalid'),
      confirmButtonText: t('common.confirm'),
      cancelButtonText: t('common.cancel'),
    })
    next = String(res.value ?? '').trim()
  } catch {
    return // 取消（留空 / 不合规时弹窗自己就不让确认）
  }
  if (!next || next === from) return

  try {
    // 现查一次（不是查本地列表）：重名后端也会拒绝，但报的是「目标已存在：xxx」
    const taken = await projectExists(next)
    if (taken.exists) {
      ElMessage.warning(t('home.duplicateExists', { name: next }))
      return
    }
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
    return
  }

  try {
    await copyEntry(from, next)

    const manifest = await readFile(`${next}/manifest`)
    // 不带桌面入口的项目没有这份 config，读不到就是「没有」，不是错误
    const config = await readFile(`${next}/app/ui/config`).catch(() => null)
    const rewrite = rewriteForDuplicate(next, manifest.content, config?.content ?? null)
    await writeFile(`${next}/manifest`, rewrite.manifest)
    if (rewrite.uiConfig !== null) await writeFile(`${next}/app/ui/config`, rewrite.uiConfig)
    // 解析不了的 config 不写回（见 utils/duplicate.ts）：说一声，别让人以为改好了
    if (rewrite.uiConfigSkipped === 'invalid') ElMessage.warning(t('home.duplicateConfigSkipped'))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
    return
  }

  ElMessage.success(t('home.duplicated', { name: next }))
  await projects.refresh()
}

async function remove() {
  const name = props.item.name
  const label = props.item.displayName || name
  try {
    await ElMessageBox.confirm(t('home.confirmDelete', { name: label }), t('common.tip'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return // 取消
  }
  try {
    await projects.remove(name)
    // 列表已经刷新过了（这张卡片多半也已经被卸载），后面只剩全局动作
    tabs.closeProject(name)
    ElMessage.success(t('home.deleted', { name }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}
</script>
