<template>
  <div class="cm-host h-full min-h-0 overflow-hidden" :class="{ 'cm-mobile': layout.isMobile }">
    <!--
      :key 跟主题走：扩展在 CodeMirror 里是追加进去的，切到浅色时没法把 oneDark
      摘下来。换主题就重建一次编辑器（内容由 v-model 带过去），代价是光标与撤销栈，
      但主题切换本来就是罕见动作。
    -->
    <Codemirror
      :key="theme.resolved"
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
import { useThemeStore } from '@/stores/theme'
import { checkFormat } from '@/utils/validate'
import type { EditorProps } from '../useEditor'

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()

/** 字号跟布局走（见 style.css 的 .cm-mobile）；换行是两种布局都要的，不在这里判。 */
const layout = useLayoutStore()

const code = ref(props.origin)
const theme = useThemeStore()

/**
 * 语言按扩展名和首行 shebang 猜。
 *
 * manifest / config 这些没有扩展名的文件也走这里——它们是 `key = value` 或
 * JSON，按 JSON 高亮比纯文本可读得多。
 *
 * 判据只看**挂载时**那份文本（`props.origin`），不看 `code`：这个计算属性一旦依赖正文，
 * 每敲一个字就会产出一份新的扩展数组，而 vue-codemirror 对扩展是**追加**的（它每次新建
 * 一个 Compartment 再 appendConfig）——结果是每敲一个键就多挂一份 [语言 + 换行 + 主题]，
 * 而且 @codemirror/language 的 language facet 取 `languages[0]`，第一个判出来的语言会
 * 永久生效（先敲 `#!/bin/bash` 再改成 JSON，高亮还是 shell）。判据固定成「打开时那份
 * 文本」，两个毛病一起没有；代价是这一份文件里现改 shebang 不会换高亮。
 */
const language = computed<Extension[]>(() => {
  const trimmed = (props.origin ?? '').trimStart()
  if (props.path.endsWith('.yaml') || props.path.endsWith('.yml')) return [yaml()]
  if (trimmed.startsWith('#!')) return [javascript()]
  if (
    trimmed.startsWith('{') ||
    trimmed.startsWith('[') ||
    /(^|\/)(manifest|privilege|resource|config)$/.test(props.path)
  ) {
    return [json()]
  }
  return []
})

const extensions = computed<Extension[]>(() => {
  // 始终按宽度自动换行：配置和脚本里那些长行横着拖很难读，桌面上也一样。
  const list: Extension[] = [EditorView.lineWrapping, ...language.value]
  // 主题跟应用主题走（以前无条件 oneDark，浅色主题下就它一块深色）。
  if (theme.resolved === 'dark') list.push(oneDark)
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
  setContent: (next: string) => {
    code.value = next
  },
  getText: () => code.value,
})
</script>
