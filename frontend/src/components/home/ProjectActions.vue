<template>
  <el-dropdown trigger="click" placement="bottom-end" @command="run">
    <!-- 下拉自身要能收到点击，所以 stop 放在外层：容器替它挡住行 / 卡片的点击 -->
    <span class="inline-flex" @click.stop>
      <el-button text size="small" :icon="MoreFilled" :title="t('home.more')" />
    </span>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item command="rename" :icon="EditPen">
          {{ t('common.rename') }}
        </el-dropdown-item>
        <el-dropdown-item command="export" :icon="Download">
          {{ t('home.export') }}
        </el-dropdown-item>
        <el-dropdown-item command="delete" :icon="Delete" divided>
          {{ t('common.delete') }}
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Download, EditPen, MoreFilled } from '@element-plus/icons-vue'
import { downloadEntry, renameEntry } from '@/api/endpoints'
import { useProjectsStore } from '@/stores/projects'
import { useTabsStore } from '@/stores/tabs'
import { useWorkspaceStore } from '@/stores/workspace'
import type { ProjectInfo } from '@/api/types'

/**
 * 项目卡片右上角的「···」菜单：重命名 / 导出 / 删除。
 *
 * 三个动作整条流程都在这里走完（改目录 → 处理项目标签 → 播报 → 最后刷列表）。
 * 顺序不能反：`projects.refresh()` 会换掉列表，外层 v-for 的 key 跟着变，
 * 这张卡片会被卸载——刷新之后再往外 emit 事件、或者再做依赖组件状态的事就来不及了。
 * 所以刷列表永远是最后一步。
 *
 * 删除只有这一条路，并且一定要过一遍确认弹窗——项目目录是直接删掉的，没有回收站。
 * 重命名改的是**项目目录名**（应用标识在 manifest.appname 里，不在这里动）。
 */
const props = defineProps<{ item: ProjectInfo }>()

const { t } = useI18n()
const projects = useProjectsStore()
const tabs = useTabsStore()
const workspace = useWorkspaceStore()

/** 目录名规则与 fnpack create 保持一致：字母数字开头，可含 . _ -。 */
const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/

function run(command: string) {
  if (command === 'rename') void rename()
  else if (command === 'export') exportProject()
  else if (command === 'delete') void remove()
}

/** 导出：把项目目录打成 zip 交给浏览器下载（后端对目录就是这么处理的）。 */
function exportProject() {
  downloadEntry(props.item.name)
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
