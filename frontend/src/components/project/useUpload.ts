import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

/**
 * 上传一组文件，带进度与取消。
 *
 * 文件管理器有两个入口会调它（文件树的目录菜单、标签页顶栏的上传按钮），所以进度状态、
 * 取消和成功 / 失败的文案都在这里写一次：两边各写一份的话，迟早有一边忘了更新。
 *
 * 进度**不再用全屏 ElLoading**：那一层遮罩会挡住所有点击，取消按钮按不到。现在进度与
 * 取消按钮由调用方就地渲染（`busy` / `percent` / `cancel`），和上传按钮在同一行。
 *
 * 取消走 AbortSignal：XHR 立刻 abort，后端的 ctx 跟着取消（见 server 的
 * handleFsUpload），不会留下一个写了一半的文件。
 */
export function useUpload() {
  const { t } = useI18n()
  const workspace = useWorkspaceStore()

  /** 当前这次上传的取消手柄；不在上传时为 null。 */
  let controller: AbortController | null = null
  const busy = ref(false)
  const percent = ref(0)

  async function upload(project: string, dir: string, files: File[]): Promise<boolean> {
    if (!files.length) return false
    // 一次只传一批：并发上传会把进度条和取消按钮指向两个不同的请求。
    if (busy.value) return false

    const ctl = new AbortController()
    controller = ctl
    busy.value = true
    percent.value = 0
    try {
      // 同名覆盖：上传是用户明确挑的文件，再问一次「要不要覆盖」没有意义，
      // 但失败的原因（权限、目录不存在、超过 1G…）要原样报出来。
      const res = await workspace.uploadTo(
        project,
        dir,
        files,
        true,
        (p) => {
          percent.value = p
        },
        ctl.signal,
      )
      ElMessage.success(t('tree.uploadDone', { count: res.files?.length ?? files.length }))
      return true
    } catch (e) {
      // 用户自己点的取消不是错误
      if (ctl.signal.aborted) ElMessage.info(t('tree.uploadCanceled'))
      else ElMessage.error(t('tree.uploadFailed', { error: e instanceof Error ? e.message : String(e) }))
      return false
    } finally {
      if (controller === ctl) controller = null
      busy.value = false
      percent.value = 0
    }
  }

  /** 取消正在进行的上传（没有在传时什么都不做）。 */
  function cancel() {
    controller?.abort()
  }

  return { upload, cancel, busy, percent }
}
