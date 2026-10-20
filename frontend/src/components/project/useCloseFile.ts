import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

/**
 * 关闭一个文件标签。
 *
 * 桌面的文件子标签、移动端的文件子页面是「同一个文件标签的两个入口」，关之前都要
 * 先问一句未保存的改动，所以这段确认逻辑单独拎出来给两边共用——两处各写一份的话，
 * 迟早有一边忘了问。
 */
export function useCloseFile() {
  const { t } = useI18n()
  const workspace = useWorkspaceStore()

  /**
   * 关掉这个文件标签；有未保存改动时先弹确认，用户取消就什么都不做。
   *
   * `adjacent` 只在桌面的标签栏传：关掉之后停在左边的相邻标签上（由 store 决定），
   * 移动端子页面的「后退」不传，仍旧退回打开文件之前的位置。
   */
  async function closeFile(project: string, path: string, opts: { adjacent?: boolean } = {}) {
    const file = workspace.state(project).files.find((f) => f.path === path)
    if (file?.dirty) {
      try {
        await ElMessageBox.confirm(t('tab.closeDirty', { name: file.name }), t('common.tip'), {
          type: 'warning',
          confirmButtonText: t('common.confirm'),
          cancelButtonText: t('common.cancel'),
        })
      } catch {
        return
      }
    }
    workspace.closeFile(project, path, opts.adjacent === true)
  }

  return { closeFile }
}
