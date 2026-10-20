import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

/**
 * 下载文件 / 目录，失败时给一句提示。
 *
 * 必须有人接住失败：下载现在是 `fetch`（见 `api/client.ts` 的 `download`），失败会
 * reject。四个入口（文件树菜单、标签页菜单、总览的产物列表、首页卡片的「导出」）共用
 * 这一段——各写一份的话，迟早有一边忘了 catch，表现是「点了没反应」加一条 unhandled
 * rejection。
 *
 * `zip: true` 表示这次下的是**目录**：后端要先把它整个打进一个临时 zip 再发正文
 * （见 `handleFsDownload`），稍大一点的目录能等好几秒，界面此时一点动静都没有——
 * 用户会以为没点中，于是再点几次（每次都在后端排一份压缩）。所以这种下载挂一条
 * **不自动消失**的提示，拿到响应（或失败）时关掉。单文件下载不提示：它几乎是瞬时的，
 * 弹一条又立刻消失的提示只是闪一下。
 */
export function useDownload() {
  const { t } = useI18n()
  const workspace = useWorkspaceStore()

  async function download(
    path: string,
    opts: { zip?: boolean; package?: boolean } = {},
  ): Promise<void> {
    // duration: 0 = 不自动关闭，由下面 finally 里的 close() 收尾
    const tip = opts.zip ? ElMessage({ message: t('tree.downloadZip'), duration: 0 }) : null
    try {
      await workspace.downloadAt(path, { package: opts.package })
    } catch (e) {
      ElMessage.error(
        t('tree.downloadFailed', { error: e instanceof Error ? e.message : String(e) }),
      )
    } finally {
      tip?.close()
    }
  }

  return { download }
}
