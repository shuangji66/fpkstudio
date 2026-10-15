import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useProjectsStore } from '@/stores/projects'
import { isHostAvailable, openInFileManager } from '@/trim/sdk'
import { trimErrorMessage } from '@/trim/errors'

/**
 * 「在文件管理器里打开项目目录」。
 *
 * 三个入口共用：首页卡片的文件夹按钮（`ProjectActions`）、左栏文件树顶栏、以及「文件」
 * 标签页顶栏。三处各写一份的话，迟早有一处忘了判环境或者忘了 catch——宿主的
 * `openFileManager` 会 reject，表现是「点了没反应」加一条 unhandled rejection。
 *
 * 传项目名而不是绝对路径：路径只有后端知道（项目根可以被换成授权目录，
 * `projectsDir + name` 是拼不出来的，见 `/api/projects` 的 `absPath`）。项目列表还没加载
 * 出来时按钮置灰。
 */
export function useOpenFolder(project: () => string) {
  const { t, te } = useI18n()
  const projects = useProjectsStore()

  const folderPath = computed(() => projects.find(project())?.absPath ?? '')

  /** 文件管理器是宿主能力：独立浏览器页面里没有它可开。 */
  const canOpenFolder = computed(() => isHostAvailable() && Boolean(folderPath.value))

  async function openFolder(): Promise<void> {
    if (!folderPath.value) return
    try {
      await openInFileManager(folderPath.value)
    } catch (e) {
      ElMessage.error(trimErrorMessage(t, te, e))
    }
  }

  return { canOpenFolder, openFolder }
}
