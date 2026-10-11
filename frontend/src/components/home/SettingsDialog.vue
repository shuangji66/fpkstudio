<template>
  <el-dialog
    v-model="open"
    :title="t('settings.title')"
    class="settings-dialog"
    modal-class="settings-modal"
    width="640px"
    align-center
    lock-scroll
    append-to-body
    @open="onOpen"
  >
    <div class="flex flex-col gap-5">
      <!-- 主题 -->
      <section>
        <p class="label-text mb-2">{{ t('settings.theme.title') }}</p>
        <el-radio-group :model-value="theme.mode" @update:model-value="onTheme">
          <el-radio-button value="light">{{ t('settings.theme.light') }}</el-radio-button>
          <el-radio-button value="dark">{{ t('settings.theme.dark') }}</el-radio-button>
          <el-radio-button value="system">{{ t('settings.theme.system') }}</el-radio-button>
        </el-radio-group>
        <p class="mt-2 text-[11px] leading-relaxed text-slate-500 dark:text-slate-400">
          {{ themeHint }}
        </p>
      </section>

      <!-- 目录设置 -->
      <section>
        <div class="mb-2 flex items-center gap-2">
          <p class="label-text">{{ t('settings.dirs.title') }}</p>
          <span class="flex-1" />
          <span
            v-if="info && !info.trim.available"
            class="text-[11px] text-amber-600 dark:text-amber-400"
          >
            {{ info.trim.message }}
          </span>
        </div>

        <div class="flex flex-col gap-3">
          <DirSlot
            slot="projectRoot"
            :path="info?.dirs.projectRoot ?? ''"
            :from-user="info?.dirs.projectRootFromUser ?? false"
            :default-path="info?.dirs.defaultProjectRoot ?? ''"
            :semantic="semanticOf('projectRoot')"
            :busy="busy"
            :pick-disabled="busy || !hostAvailable"
            :pick-title="hostAvailable ? '' : t('trim.error.hostUnavailable')"
            :can-open-folder="canOpenFolder(info?.dirs.projectRoot)"
            @pick="pick('projectRoot')"
            @clear="clear('projectRoot')"
            @open-folder="openFolder(info?.dirs.projectRoot)"
          />
          <DirSlot
            slot="outputDir"
            :path="info?.dirs.outputDir ?? ''"
            :from-user="info?.dirs.outputDirFromUser ?? false"
            :default-path="info?.dirs.defaultOutputDir ?? ''"
            :env-ignored="info?.dirs.outputEnvIgnored ?? false"
            :semantic="semanticOf('outputDir')"
            :busy="busy"
            :pick-disabled="busy || !hostAvailable"
            :pick-title="hostAvailable ? '' : t('trim.error.hostUnavailable')"
            :can-open-folder="canOpenFolder(info?.dirs.outputDir)"
            @pick="pick('outputDir')"
            @clear="clear('outputDir')"
            @open-folder="openFolder(info?.dirs.outputDir)"
          />
        </div>

        <p
          v-if="semanticsError"
          class="mt-2 text-[11px] leading-relaxed text-amber-600 dark:text-amber-400"
        >
          {{ t('settings.dirs.semanticFailed', { error: semanticsError }) }}
        </p>
      </section>

      <!--
        关于：仓库地址是写死的常量，不做成配置——它就是「这个程序是哪个」这个事实，
        没有第二个取值。
      -->
      <section class="border-t border-slate-200 pt-3 dark:border-slate-700">
        <div class="flex items-center gap-2">
          <span class="label-text">{{ t('settings.about.title') }}</span>
          <a
            :href="REPO_URL"
            target="_blank"
            rel="noopener noreferrer"
            class="text-slate-400 transition-colors hover:text-slate-700 dark:text-slate-500 dark:hover:text-slate-200"
            :title="t('settings.about.repo')"
            :aria-label="t('settings.about.repo')"
          >
            <!-- Element Plus 没有 GitHub 图标，这里直接内联官方 mark（16×16） -->
            <svg viewBox="0 0 16 16" width="16" height="16" fill="currentColor" aria-hidden="true">
              <path
                d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.42 7.42 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"
              />
            </svg>
          </a>
        </div>
        <p class="mt-1.5 text-[11px] leading-relaxed text-slate-500 dark:text-slate-400">
          {{ t('settings.about.desc') }}
        </p>
      </section>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import DirSlot from './SettingsDirSlot.vue'
import {
  convertPaths,
  deleteAuthorizedFolder,
  getAuthorizedFolders,
  getAppSettings,
  saveAppSettings,
} from '@/api/endpoints'
import type { SettingsInfo } from '@/api/types'
import { isHostAvailable, openInFileManager, pickUserDirectory } from '@/trim/sdk'
import { trimErrorMessage } from '@/trim/errors'
import { indexSemantics } from '@/utils/semantic'
import { useThemeStore, type ThemeMode } from '@/stores/theme'

type Slot = 'projectRoot' | 'outputDir'

/** 仓库地址：关于一节里的图标链到这里。 */
const REPO_URL = 'https://github.com/shuangji66/fpkstudio'
type Translate = (key: string, named?: Record<string, unknown>) => string

const open = defineModel<boolean>({ required: true })

/** 目录设置真的变了才通知外面刷新项目列表（主题变了不用）。 */
const emit = defineEmits<{ saved: [] }>()

const { t, te, locale } = useI18n()
const theme = useThemeStore()

const info = ref<SettingsInfo | null>(null)
const semantics = ref<Record<string, string>>({})
/** 语义路径转换失败的原因；为空表示没出错（或压根没跑在飞牛里）。 */
const semanticsError = ref('')
const busy = ref(false)

const hostAvailable = computed(() => isHostAvailable())

const themeHint = computed(() => {
  if (theme.mode !== 'system') {
    return t('settings.theme.hintFixed', { theme: t(`settings.theme.${theme.resolved}`) })
  }
  return theme.hostThemeValue
    ? t('settings.theme.hintHost', { theme: t(`settings.theme.${theme.resolved}`) })
    : t('settings.theme.hintBrowser', { theme: t(`settings.theme.${theme.resolved}`) })
})

const fail = (err: unknown): string => trimErrorMessage(t as Translate, te, err)

/** 目录能不能在文件管理器里打开：宿主能力 + 有路径（默认目录也算有）。 */
const canOpenFolder = (path: string | undefined): boolean => hostAvailable.value && Boolean(path)

async function openFolder(path: string | undefined) {
  if (!path) return
  try {
    await openInFileManager(path)
  } catch (e) {
    ElMessage.error(fail(e))
  }
}

function onTheme(value: string | number | boolean | undefined) {
  theme.setMode(String(value) as ThemeMode)
}

const semanticOf = (slot: Slot): string => {
  const path = slot === 'projectRoot' ? info.value?.dirs.projectRoot : info.value?.dirs.outputDir
  return path ? semantics.value[path] ?? '' : ''
}

async function onOpen() {
  await reload()
}

/** 拉一次设置，并把两条路径的权限与语义路径一起问出来。 */
async function reload() {
  try {
    info.value = await getAppSettings()
  } catch (e) {
    ElMessage.error(fail(e))
    return
  }
  await refreshPathInfo()
}

/**
 * 把两条路径换成用户看得懂的展示路径（`trim.file.path`）。
 *
 * 转换失败时不再默默退回原始路径：设置没跑在飞牛里（接口本来就调不通）时安静处理，
 * 其他失败（scope 没声明、status 非 0…）在界面上给一句原因——不然用户看到的只是
 * 「语义路径没显示」，完全无从下手。
 */
async function refreshPathInfo() {
  semantics.value = {}
  semanticsError.value = ''

  const paths = [info.value?.dirs.projectRoot ?? '', info.value?.dirs.outputDir ?? ''].filter(
    Boolean,
  )
  if (!paths.length || !info.value?.trim.available) return

  try {
    semantics.value = indexSemantics(paths, await convertPaths(paths, locale.value))
  } catch (e) {
    semanticsError.value = fail(e)
  }
}

/**
 * 添加 / 更换某一栏的目录。
 *
 * 「换目录要先删掉旧目录的授权」——授权是加在系统里的，只改配置不删授权的话，
 * 一堆不再使用的目录会一直留在应用权限里。但删除必须发生在**拿到新目录之后**：
 * 先删再选的话，用户取消选择、或者选择器直接报错，旧目录的授权就没了，而配置
 * 里还写着它——应用会突然读不到自己的项目目录。
 *
 * 顺序：确认 → 拉起选择器 → 删旧授权（仅当真的换了目录）→ 落新配置。
 */
async function pick(slot: Slot) {
  if (!hostAvailable.value) {
    ElMessage.warning(t('trim.error.hostUnavailable'))
    return
  }
  if (info.value && !info.value.trim.available) {
    ElMessage.warning(info.value.trim.message)
    return
  }

  const current = slot === 'projectRoot' ? info.value?.dirs.projectRoot : info.value?.dirs.outputDir
  const fromUser =
    slot === 'projectRoot' ? info.value?.dirs.projectRootFromUser : info.value?.dirs.outputDirFromUser

  busy.value = true
  try {
    if (fromUser && current && !(await confirmReplace(current))) return

    // 目录授权只支持单选，但返回的是「当前用户授权给应用的目录」全集，所以拿选
    // 之前的那一份 diff 出新增的那个；diff 不出来时退回最后一个。列表现拉一次：
    // 弹窗打开之后用户可能已经删过授权，缓存里那份就不准了。
    const before = new Set(await getAuthorizedFolders().catch(() => []))
    const picked = await pickUserDirectory(
      t('settings.dirs.pickerTitle'),
      t('settings.dirs.pickerOk'),
    )
    // 空数组 = 用户没选就关掉了选择器（见 trim/sdk.ts 的说明），不是错误。
    if (!picked.length) {
      ElMessage.info(t('settings.dirs.canceled'))
      return
    }

    const added = picked.filter((p) => !before.has(p))
    const target = added[0] ?? picked[picked.length - 1]
    if (!target) {
      ElMessage.info(t('settings.dirs.canceled'))
      return
    }
    if (target === current) {
      ElMessage.info(t('settings.dirs.unchanged'))
      return
    }

    // 顺序不能反：先把新目录落成配置（save 里会连后端一起切过去），确认切成功了
    // 再删旧授权。反过来的话，切换失败时旧授权已经没了，应用会读不到自己的目录。
    await save(slot, target)
    await dropAuthorization(current, fromUser)
  } catch (e) {
    ElMessage.error(fail(e))
  } finally {
    busy.value = false
  }
}

/**
 * 删掉一条不再使用的目录授权。
 *
 * 授权是加在系统里的，只改配置不删授权的话，一堆用不到的目录会一直留在应用权限里，
 * 所以换目录 / 取消设置都要顺手删掉。但这一步永远是**最后**做的：它失败了最多是
 * 留一条多余的授权，而放在切换之前就会变成「配置指着一个没有权限的目录」。
 */
async function dropAuthorization(path: string | undefined, fromUser: boolean | undefined) {
  if (!fromUser || !path || !hostAvailable.value || !info.value?.trim.available) return
  try {
    await deleteAuthorizedFolder(path)
  } catch (e) {
    ElMessage.warning(fail(e))
  }
}

/** 取消设置（并删掉授权），回到环境变量给的默认目录。 */
async function clear(slot: Slot) {
  const current = slot === 'projectRoot' ? info.value?.dirs.projectRoot : info.value?.dirs.outputDir
  const fromUser =
    slot === 'projectRoot' ? info.value?.dirs.projectRootFromUser : info.value?.dirs.outputDirFromUser

  try {
    if (fromUser && current && hostAvailable.value && info.value?.trim.available) {
      await confirmClear(current)
    }
    await save(slot, '')
    await dropAuthorization(current, fromUser)
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(fail(e))
  }
}

async function save(slot: Slot, path: string) {
  const patch = slot === 'projectRoot' ? { projectRoot: path } : { outputDir: path }
  info.value = await saveAppSettings(patch)
  await refreshPathInfo()
  emit('saved')
  ElMessage.success(t('settings.dirs.saved'))
}

async function confirmReplace(path: string): Promise<boolean> {
  try {
    await ElMessageBox.confirm(
      t('settings.dirs.replaceConfirm', { path }),
      t('settings.dirs.replaceTitle'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
    return true
  } catch {
    return false
  }
}

async function confirmClear(path: string): Promise<void> {
  await ElMessageBox.confirm(
    t('settings.dirs.clearConfirm', { path }),
    t('settings.dirs.clearTitle'),
    { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
  )
}
</script>
