import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

/**
 * 下载文件 / 目录，失败时给一句提示。
 *
 * 必须有人接住失败：下载现在是 `fetch`（见 `api/client.ts` 的 `download`），失败会
 * reject。三个入口（文件树菜单、标签页菜单、总览的产物列表）共用这一段——各写一份
 * 的话，迟早有一边忘了 catch，表现是「点了没反应」加一条 unhandled rejection。
 */
export function useDownload() {
  const { t } = useI18n()
  const workspace = useWorkspaceStore()

  async function download(path: string): Promise<void> {
    try {
      await workspace.downloadAt(path)
    } catch (e) {
      ElMessage.error(
        t('tree.downloadFailed', { error: e instanceof Error ? e.message : String(e) }),
      )
    }
  }

  return { download }
}
