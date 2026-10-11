import { useI18n } from 'vue-i18n'
import { ElLoading, ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

/**
 * 上传一组文件，带进度提示。
 *
 * 文件管理器有两个入口会调它（文件树的目录菜单、标签页顶栏的上传按钮），所以进度
 * 提示和成功 / 失败的文案都在这里写一次：两边各写一份的话，进度条迟早有一边忘了更新。
 *
 * 进度用全屏 loading 显示——上传是「现在正在发生、别乱点」的动作，比弹个提示条更贴切；
 * 结束（成功或失败）时都要把它关掉，否则界面会一直卡在转圈上。
 */
export function useUpload() {
  const { t } = useI18n()
  const workspace = useWorkspaceStore()

  async function upload(project: string, dir: string, files: File[]): Promise<boolean> {
    if (!files.length) return false

    const loading = ElLoading.service({
      text: t('tree.uploading', { percent: 0 }),
      background: 'rgba(15, 23, 42, 0.35)',
    })
    try {
      // 同名覆盖：上传是用户明确挑的文件，再问一次「要不要覆盖」没有意义，
      // 但失败的原因（权限、目录不存在…）要原样报出来。
      const res = await workspace.uploadTo(project, dir, files, true, (percent) => {
        loading.setText(t('tree.uploading', { percent }))
      })
      ElMessage.success(t('tree.uploadDone', { count: res.files?.length ?? files.length }))
      return true
    } catch (e) {
      ElMessage.error(t('tree.uploadFailed', { error: e instanceof Error ? e.message : String(e) }))
      return false
    } finally {
      loading.close()
    }
  }

  return { upload }
}
