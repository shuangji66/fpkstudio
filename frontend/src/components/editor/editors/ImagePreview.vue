<template>
  <div class="flex h-full min-h-0 items-center justify-center overflow-auto thin-scroll bg-slate-100 dark:bg-slate-900 p-4">
    <img
      v-if="!failed"
      :src="url"
      :alt="path"
      class="max-h-full max-w-full rounded-md shadow-sm"
      @error="failed = true"
    />
    <p v-else class="text-center text-xs leading-relaxed text-slate-500 dark:text-slate-400">
      {{ t('editor.imageFailed') }}<br />
      <span class="mono text-[11px] text-slate-400 dark:text-slate-500">{{ url }}</span>
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { rawURL } from '@/api/client'
import { useWorkspaceStore } from '@/stores/workspace'
import type { EditorApi, EditorContent, EditorProps } from '../useEditor'

/**
 * 图片预览。
 *
 * 读文件接口对二进制只回 `text:false`（不带内容），所以这里不读内容，直接把
 * 下载地址塞给 <img>，由浏览器自己取——顺带也省掉一次 base64 编解码。
 *
 * 它自己不接受编辑，但会把外面传进来的文本原样托管（props.origin / original）：
 * svg 这种「既能当图看、又是文本」的文件，从源码视图切回预览时靠它保住未保存的
 * 修改和 dirty 状态；真正的图片 origin 是空串，getContent 直接返回 null，
 * 不会因为误点保存把文件写成空。
 */
const props = defineProps<EditorProps>()
const { t } = useI18n()
const workspace = useWorkspaceStore()

const failed = ref(false)

const url = computed(() => {
  const full = props.project ? `${props.project}/${props.path}` : props.path
  // 带上 assetRev：图标页刚写过的文件（或源码视图保存的 svg）要重新取，
  // 否则 <img> 会一直显示改动前那张。
  const rev = props.project ? workspace.state(props.project).assetRev : 0
  return rawURL('/api/fs/download', { path: full, rev })
})

// 地址变了（重新写入过）就把失败状态清掉，否则文件补上了也看不到。
watch(url, () => {
  failed.value = false
})

const baseline = () => props.original ?? props.origin

defineExpose({
  isDirty: () => props.origin !== baseline(),
  getContent: (): EditorContent | null =>
    props.origin ? { path: props.path, content: props.origin } : null,
} satisfies EditorApi)
</script>
