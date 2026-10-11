<template>
  <div class="cm-host h-full min-h-0 overflow-hidden">
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
import { json } from '@codemirror/lang-json'
import { yaml } from '@codemirror/lang-yaml'
import { javascript } from '@codemirror/lang-javascript'
import { oneDark } from '@codemirror/theme-one-dark'
import type { Extension } from '@codemirror/state'
import type { EditorProps } from '../useEditor'

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()

const code = ref(props.origin)

/**
 * 语言按扩展名和首行 shebang 猜。
 *
 * manifest / config 这些没有扩展名的文件也走这里——它们是 `key = value` 或
 * JSON，按 JSON 高亮比纯文本可读得多。
 */
const extensions = computed<Extension[]>(() => {
  const list: Extension[] = [oneDark]
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
