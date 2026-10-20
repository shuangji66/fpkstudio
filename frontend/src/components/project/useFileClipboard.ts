import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

/**
 * 文件管理器的剪贴板动作（复制 / 剪切 / 粘贴 / 撤销）。
 *
 * 文件树和「文件」标签页共用：两处菜单、键盘快捷键都要同一套提示与错误处理，
 * 各写一份迟早会有一边忘了处理「目标重名」这类结果。
 */
export function useFileClipboard(project: () => string) {
  const { t } = useI18n()
  const workspace = useWorkspaceStore()

  /** 复制 / 剪切一批路径；复制成功给一句提示，剪切不给（菜单里有颜色和剪贴板状态）。 */
  function copy(paths: string[], cut: boolean) {
    if (!paths.length) return
    workspace.copyToClipboard(project(), paths, cut)
    if (cut) return
    const first = paths[0]!.split('/').pop() ?? paths[0]!
    ElMessage.success(
      paths.length === 1
        ? t('tree.copied', { name: first })
        : t('tree.copiedMany', { count: paths.length }),
    )
  }

  /**
   * 粘到 destDir。目标重名时复制会自动换名、剪切整批拒绝；这里只负责把结果显示出来。
   * `invalid`（剪贴板空 / 粘进自己里面）不给提示：那是「没有这个动作」，不是失败。
   */
  async function paste(destDir: string): Promise<void> {
    const clip = workspace.state(project()).clipboard
    const count = clip?.paths.length ?? 0
    const firstName = clip?.paths[0]?.split('/').pop() ?? ''
    try {
      const outcome = await workspace.pasteInto(project(), destDir)
      if (outcome === 'exists') {
        ElMessage.warning(
          count > 1
            ? t('tree.pasteExistsMany')
            : t('tree.pasteExists', { name: firstName }),
        )
      } else if (outcome === 'same') {
        ElMessage.info(
          count > 1 ? t('tree.pasteSameMany') : t('tree.pasteSame', { name: firstName }),
        )
      } else if (outcome === 'unreadable') {
        ElMessage.error(
          t('tree.pasteListFailed', { name: destDir.split('/').pop() ?? destDir }),
        )
      }
    } catch (e) {
      ElMessage.error(e instanceof Error ? e.message : String(e))
    }
  }

  /** Ctrl+Z：没有可撤销的就静默；失败才提示（原因已经记在 state.error 里）。 */
  async function undo() {
    const outcome = await workspace.undo(project())
    if (outcome === 'failed') ElMessage.error(t('tree.undoFailed'))
  }

  return { copy, paste, undo }
}
