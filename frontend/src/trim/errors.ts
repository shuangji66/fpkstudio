/**
 * 把宿主 / 后端的授权错误翻成人话。
 *
 * 飞牛的 JSSDK 用数字业务码表达失败原因（见官方「错误码」），直接抛 `msg` 有时
 * 只是一句「调用失败」。这里按码给出可操作的提示，认不出来的码再退回原文 +
 * 码值，至少能让用户知道去哪儿查。
 */
import { TrimError, TrimUnavailableError } from './sdk'

type Translate = (key: string, named?: Record<string, unknown>) => string
type HasKey = (key: string) => boolean

export function trimErrorMessage(t: Translate, te: HasKey, err: unknown): string {
  if (err instanceof TrimUnavailableError) {
    return t(err.reason === 'notReady' ? 'trim.error.hostNotReady' : 'trim.error.hostUnavailable')
  }

  if (err instanceof TrimError) {
    if (err.code > 0) {
      const key = `trim.error.code.${err.code}`
      if (te(key)) return t(key)
      return t('trim.error.unknown', { code: err.code, msg: err.message })
    }
    // code <= 0 是「没拿到业务码」的情况（宿主没返回结果、后端只说没成功），
    // 再报一个 code 0 只会让人以为接口成功了。
    return t('trim.error.failed', { msg: err.message })
  }

  return err instanceof Error ? err.message : String(err)
}
