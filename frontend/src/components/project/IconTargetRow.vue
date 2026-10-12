<template>
  <div class="flex items-center gap-2 rounded-md border border-slate-200 dark:border-slate-700 px-2 py-1.5">
    <el-checkbox
      :model-value="checked"
      size="default"
      @update:model-value="emit('update:checked', $event === true)"
    />

    <!--
      缩略图：写过就有图，没写过就是一个空框——「缺的留空」，一眼看出哪个入口还没图标。
      地址由父组件给（带上 assetRev，刚写出去的图标不吃缓存），这里不做请求。
    -->
    <span
      class="flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded border"
      :class="src ? 'border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800' : 'border-dashed border-slate-300 dark:border-slate-600 bg-slate-50 dark:bg-slate-900'"
    >
      <img v-if="src" :src="src" alt="" class="h-full w-full object-contain" />
    </span>

    <span class="shrink-0 text-sm text-slate-700 dark:text-slate-200">{{ t(labelKey) }}</span>
    <span
      class="mono min-w-0 flex-1 truncate text-right text-sm text-slate-400 dark:text-slate-500"
      :title="path"
    >
      {{ path }}
    </span>
    <span class="mono shrink-0 text-sm text-slate-500 dark:text-slate-400">{{ size }}px</span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

/**
 * 图标页「写入到」列表里的一行：勾选 + 预览 + 名称 + 落地路径 + 实际写出的边长。
 *
 * 固定的四个位置、用户加出来的副入口组、以及从目录里认出来的副入口组用的是同一行，
 * 差别只在路径与尺寸，所以单独抽出来，免得同一段标记在父组件里重复好几遍。
 */
defineProps<{
  checked: boolean
  /** i18n 键：应用 / 入口 × 大 / 小。 */
  labelKey: string
  /** 相对项目根的落地路径。 */
  path: string
  /** 这次会写出去的像素边长（小图已经乘过小图缩放）。 */
  size: number
  /** 缩略图地址；这个位置还没写过时为空串（留空）。 */
  src?: string
}>()

const emit = defineEmits<{ 'update:checked': [value: boolean] }>()

const { t } = useI18n()
</script>
