<template>
  <div class="cm-host h-full min-h-0 overflow-hidden" :class="{ 'cm-mobile': layout.isMobile }">
    <Codemirror
      v-model="code"
      :style="{ height: '100%' }"
      :extensions="extensions"
      :indent-with-tab="true"
      :tab-size="2"
      @change="onChange"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Codemirror } from 'vue-codemirror'
import { EditorView } from 'codemirror'
import { json } from '@codemirror/lang-json'
import { yaml } from '@codemirror/lang-yaml'
import { javascript } from '@codemirror/lang-javascript'
import { oneDark } from '@codemirror/theme-one-dark'
import type { Extension } from '@codemirror/state'
import { useI18n } from 'vue-i18n'
import { useLayoutStore } from '@/stores/layout'
import { checkFormat } from '@/utils/validate'
import type { EditorProps } from '../useEditor'

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()

/** 字号跟布局走（见 style.css 的 .cm-mobile）；换行是两种布局都要的，不在这里判。 */
const layout = useLayoutStore()

const code = ref(props.origin)

/**
 * 语言按扩展名和首行 shebang 猜。
 *
 * manifest / config 这些没有扩展名的文件也走这里——它们是 `key = value` 或
 * JSON，按 JSON 高亮比纯文本可读得多。
 */
const extensions = computed<Extension[]>(() => {
  // 始终按宽度自动换行：配置和脚本里那些长行横着拖很难读，桌面上也一样。
  const list: Extension[] = [oneDark, EditorView.lineWrapping]
  const trimmed = code.value.trimStart()
  if (props.path.endsWith('.yaml') || props.path.endsWith('.yml')) {
    list.unshift(yaml())
  } else if (trimmed.startsWith('#!')) {
    list.unshift(javascript())
  } else if (
    trimmed.startsWith('{') ||
    trimmed.startsWith('[') ||
    /(^|\/)(manifest|privilege|resource|config)$/.test(props.path)
  ) {
    list.unshift(json())
  }
  return list
})

watch(
  () => props.origin,
  (value) => {
    code.value = value
    emit('dirty', false)
  },
)

const baseline = () => props.original ?? props.origin

function onChange(value: string) {
  emit('dirty', value !== baseline())
}

defineExpose({
  isDirty: () => code.value !== baseline(),
  /**
   * 保存前的格式校验（EditorHost 会先问这个）。
   *
   * 表单视图有各自的校验，源码视图没有——它能直接把文本写进文件，写坏的 JSON
   * 会让那个文件在 fnOS 里失效。拦下来时只弹提示、**不动编辑器里的内容**：
   * 用户刚敲的东西不能因为格式不对就没了。
   */
  saveBlocked: () => {
    const issue = checkFormat(props.path, code.value)
    return issue ? t(issue.key, issue.params ?? {}) : ''
  },
  getContent: () => ({ path: props.path, content: code.value }),
  reset: () => {
    code.value = props.origin
    emit('dirty', false)
  },
  setContent: (next: string) => {
    code.value = next
  },
  getText: () => code.value,
})
</script>
