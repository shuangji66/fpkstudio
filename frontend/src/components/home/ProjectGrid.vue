<template>
  <!--
    列表加载失败要说出来。以前失败也走「没有项目」那一支，用户看到的是
    「还没有项目。用左上角的『新建项目』…」——明明项目都在，只是接口没通。
  -->
  <div v-if="projects.error" class="panel p-8 text-center text-sm">
    <p class="break-all leading-relaxed text-red-600 dark:text-red-400">
      {{ t('home.listFailed', { error: projects.error }) }}
    </p>
    <el-button text size="default" class="mt-2" :icon="Refresh" @click="projects.refresh()">
      {{ t('common.retry') }}
    </el-button>
  </div>

  <div v-else-if="projects.loading && !projects.count" class="panel p-8 text-center text-sm text-slate-500 dark:text-slate-400">
    {{ t('common.loading') }}
  </div>

  <div v-else-if="!projects.count" class="panel p-8 text-center text-sm text-slate-500 dark:text-slate-400">
    {{ t('home.empty') }}
  </div>

  <div v-else-if="!projects.filtered.length" class="panel p-8 text-center text-sm text-slate-500 dark:text-slate-400">
    {{ t('home.noMatch') }}
  </div>

  <!--
    卡片网格。两种视图共用同一张卡：

    - 平铺：`auto-fill` 按宽度自己分列，项目多了整页纵向滚。
    - 分页：列数 / 行数由外面按容器尺寸算好（见 utils/paging.ts），行高 `1fr` 撑满
      容器高度，所以一页正好铺满、不多不少；`items` 只给这一页的项目。
  -->
  <div v-else class="grid gap-3" :class="fit ? '' : 'grid-cols-[repeat(auto-fill,minmax(17rem,1fr))]'" :style="fit ? pagedStyle : undefined">
    <ProjectCard
      v-for="item in items"
      :key="item.name"
      :item="item"
      @open="emit('open', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@element-plus/icons-vue'
import ProjectCard from './ProjectCard.vue'
import { useProjectsStore } from '@/stores/projects'
import type { GridFit } from '@/utils/paging'
import type { ProjectInfo } from '@/api/types'

const props = defineProps<{
  /** 这一屏要渲染的项目（平铺 = 全部，分页 = 当前页）。 */
  items: ProjectInfo[]
  /** 分页视图的排版结果；不给就按平铺（auto-fill 自己分列）渲染。 */
  fit?: GridFit | null
}>()

const emit = defineEmits<{ open: [project: ProjectInfo] }>()

const { t } = useI18n()
const projects = useProjectsStore()

/**
 * 分页视图的网格：列数写死成算好的值，行数用 `1fr` 等分容器高度。
 *
 * 行高必须均分而不是按内容：卡片高度差一行文字（有没有 mono 的完整项目名），
 * 按内容排的话最后一行会被容器切掉一半。写 inline style 是因为列数是运行时算的，
 * `grid-cols-${n}` 这种拼出来的类名 Tailwind 扫不到（见 AGENTS 里的记录）。
 */
const pagedStyle = computed(() => {
  const fit = props.fit
  if (!fit) return undefined
  return {
    gridTemplateColumns: `repeat(${fit.cols}, minmax(0, 1fr))`,
    gridTemplateRows: `repeat(${fit.rows}, minmax(0, 1fr))`,
    height: '100%',
  }
})
</script>
