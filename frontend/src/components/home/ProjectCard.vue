<template>
  <article
    class="panel group flex cursor-pointer flex-col p-3 transition-shadow hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400"
    :title="t('home.openHint', { name: projectLabel(item) })"
    role="button"
    tabindex="0"
    :aria-label="`${t('home.open')} ${projectLabel(item)}`"
    @click="emit('open', item)"
    @keydown.enter.prevent="emit('open', item)"
    @keydown.space.prevent="emit('open', item)"
  >
    <!-- 项目名一行：· 标题 + 类型标签，「···」永远在这一行的右上角 -->
    <div class="flex min-w-0 flex-1 items-start gap-2">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <h3 class="mono truncate text-sm font-semibold text-slate-800 dark:text-slate-100" :title="item.name">
            {{ projectLabel(item) }}
          </h3>
          <!-- 只在需要提醒时打标签：docker、以及没有桌面入口的原生项目 -->
          <el-tag v-if="kindLabel" class="text-sm" :type="kindType" effect="plain">
            {{ kindLabel }}
          </el-tag>
        </div>
        <p
          v-if="item.name !== projectLabel(item)"
          class="mono mt-0.5 truncate text-sm text-slate-500 dark:text-slate-400"
          :title="item.name"
        >
          {{ item.name }}
        </p>
      </div>

      <ProjectActions :item="item" />
    </div>

    <dl class="mt-2 grid grid-cols-[4rem_1fr] gap-x-2 gap-y-0.5 text-sm">
      <dt class="text-slate-400 dark:text-slate-500">{{ t('home.modifiedAt') }}</dt>
      <dd class="truncate text-slate-600 dark:text-slate-300">{{ item.modTime }}</dd>
    </dl>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ProjectActions from './ProjectActions.vue'
import { projectLabel } from '@/utils/format'
import type { ProjectInfo } from '@/api/types'

/**
 * 项目卡片。
 *
 * 首页只有这一种卡片（平铺 / 分页两种视图共用同一张卡）：**点卡片即打开项目**
 * （没有单独的「打开」按钮——卡片就那么点地方，一个占满整行的按钮只是把点击目标变小了）。
 * 重命名 / 导出 / 删除在 ProjectActions 里，打开项目目录也在那儿（「···」左边的文件夹
 * 按钮），它们自己走完整条流程，所以这里只需要把「打开」抛给上层。
 *
 * 卡片本身可点，所以内部按钮都要自己挡住冒泡（ProjectActions 里统一 stop），
 * 键盘用户按 Enter / 空格也能打开。
 */
const props = defineProps<{ item: ProjectInfo }>()

const emit = defineEmits<{ open: [project: ProjectInfo] }>()

const { t } = useI18n()

/**
 * 卡片上的类型标签。
 *
 * 只在「需要知道」时才显示：docker 项目、以及没有桌面入口的原生项目。
 * 原生 + 入口是最常见的情况，再挂个「原生」标签只是噪声。
 */
const kindLabel = computed(() => {
  if (props.item.docker) return t('project.kindDocker')
  if (!props.item.ui) return t('project.kindNoEntry')
  return ''
})

const kindType = computed<'warning' | 'info'>(() => (props.item.docker ? 'warning' : 'info'))
</script>
