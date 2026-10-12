<template>
  <!--
    列表加载失败要说出来。以前失败也走「没有项目」那一支，用户看到的是
    「还没有项目。用左上角的『新建项目』…」——明明项目都在，只是接口没通。
  -->
  <div v-if="projects.error" class="panel p-8 text-center text-sm">
    <p class="break-all leading-relaxed text-red-600 dark:text-red-400">
      {{ t('home.listFailed', { error: projects.error }) }}
    </p>
    <el-button text size="small" class="mt-2" :icon="Refresh" @click="projects.refresh()">
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

  <!-- 网格：卡片自适应列宽 -->
  <div
    v-else-if="view === 'grid'"
    class="grid grid-cols-[repeat(auto-fill,minmax(17rem,1fr))] gap-3"
  >
    <ProjectCard
      v-for="item in projects.filtered"
      :key="item.name"
      layout="grid"
      :item="item"
      @open="emit('open', $event)"
    />
  </div>

  <!--
    列表：一行一个项目。宽度足够时按固定列宽分列（先填满一列的高度、再横向铺开，
    横向滚动）；窄屏只剩一列，纵向滚动。列数规则在 style.css 的 .list-columns 里。
  -->
  <div v-else class="list-columns gap-3">
    <ProjectCard
      v-for="item in projects.filtered"
      :key="item.name"
      layout="list"
      :item="item"
      @open="emit('open', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Refresh } from '@element-plus/icons-vue'
import ProjectCard from './ProjectCard.vue'
import { useProjectsStore } from '@/stores/projects'
import type { ProjectInfo } from '@/api/types'

defineProps<{ view: 'grid' | 'list' }>()

const emit = defineEmits<{ open: [project: ProjectInfo] }>()

const { t } = useI18n()
const projects = useProjectsStore()
</script>
