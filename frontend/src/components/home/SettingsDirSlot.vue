<template>
  <div class="rounded-lg border border-slate-200 p-2.5 dark:border-slate-700">
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs font-medium text-slate-700 dark:text-slate-200">
        {{ t(`settings.dirs.${slot}.title`) }}
      </span>
      <el-tag size="small" :type="fromUser ? 'success' : 'info'" effect="plain">
        {{ fromUser ? t('settings.dirs.fromUser') : t('settings.dirs.default') }}
      </el-tag>

      <span class="flex-1" />

      <el-button
        text
        size="small"
        :icon="FolderOpened"
        :disabled="!canOpenFolder"
        :title="canOpenFolder ? t('settings.dirs.openFolder') : t('trim.error.hostUnavailable')"
        @click="emit('openFolder')"
      />
      <el-button size="small" :disabled="pickDisabled" :title="pickTitle" @click="emit('pick')">
        {{ fromUser ? t('settings.dirs.replace') : t('settings.dirs.add') }}
      </el-button>
      <el-button
        v-if="fromUser"
        size="small"
        text
        type="danger"
        :disabled="busy"
        @click="emit('clear')"
      >
        {{ t('settings.dirs.clear') }}
      </el-button>
    </div>

    <!--
      只显示 trim.file.path 转换出来的展示路径（「存储空间1/admin 的文件/fpk」）：
      /vol1/... 这种内部路径对用户没有意义。转换失败或没跑在飞牛里时才回落显示原始
      路径（这时 dialog 会另外给一句失败原因）。
    -->
    <p
      class="mt-1.5 truncate text-xs text-slate-700 dark:text-slate-200"
      :title="semantic || path"
    >
      {{ semantic || path }}
    </p>

    <!-- 环境变量被忽略的情况要说出来，否则用户会以为「我明明设了输出目录」 -->
    <p
      v-if="envIgnored"
      class="mt-1.5 text-[11px] leading-relaxed text-amber-600 dark:text-amber-400"
    >
      {{ t('settings.dirs.envIgnored', { path: defaultPath }) }}
    </p>
    <p v-else class="mt-1.5 text-[11px] text-slate-400 dark:text-slate-500">
      {{
        fromUser ? t('settings.dirs.defaultHint', { path: defaultPath }) : t('settings.dirs.notSet')
      }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { FolderOpened } from '@element-plus/icons-vue'

defineProps<{
  slot: 'projectRoot' | 'outputDir'
  /** 当前生效的绝对路径（配置里存着的那个）。 */
  path: string
  /** 是否来自用户设置（否则就是环境变量给的默认目录）。 */
  fromUser: boolean
  /** 默认目录，用于告诉用户「取消设置会回到哪里」。 */
  defaultPath: string
  /** 转换后的展示路径（trim.file.path）；拿不到时为空。 */
  semantic: string
  busy: boolean
  /** 选目录按钮是否禁用（比如页面没跑在飞牛宿主里）。 */
  pickDisabled: boolean
  /** 禁用原因，鼠标悬停时显示。 */
  pickTitle: string
  /** 环境变量指到了和项目目录相同的位置、被忽略（只可能出现在输出目录这一槽）。 */
  envIgnored?: boolean
  /** 能不能在文件管理器里打开（宿主能力 + 有路径）。 */
  canOpenFolder: boolean
}>()

const emit = defineEmits<{ pick: []; clear: []; openFolder: [] }>()

const { t } = useI18n()
</script>
