<template>
  <div v-if="tab" class="flex h-full min-h-0 flex-col bg-white">
    <div class="flex shrink-0 items-center gap-2 border-b border-slate-200 px-2 py-1">
      <el-icon class="shrink-0 text-slate-400"><Document /></el-icon>
      <span class="mono shrink-0 text-[11px] text-slate-500">{{ tab.path }}</span>
      <span class="shrink-0 text-[10px] text-slate-400">{{ tab.sizeRaw }}</span>
      <span v-if="tab.dirty" class="shrink-0 text-[11px] text-amber-600">● {{ t('editor.unsaved') }}</span>

      <span class="flex-1" />

      <el-radio-group v-if="def.hasForm && tab.text" v-model="mode" size="small">
        <el-radio-button value="form">{{ t('editor.form') }}</el-radio-button>
        <el-radio-button value="source">{{ t('editor.source') }}</el-radio-button>
      </el-radio-group>

      <el-button size="small" :disabled="!tab.dirty" @click="revert">
        {{ t('editor.revert') }}
      </el-button>
      <el-button size="small" type="primary" :disabled="!tab.dirty" :loading="saving" @click="save">
        {{ saving ? t('editor.saving') : t('editor.save') }}
      </el-button>
    </div>

    <div
      v-if="tab.error"
      class="shrink-0 border-b border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800"
    >
      {{ tab.error }}
    </div>

    <div v-if="tab.loading" class="flex flex-1 items-center justify-center text-xs text-slate-400">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="!tab.text" class="flex flex-1 flex-col items-center justify-center gap-3">
      <p class="text-xs text-slate-500">{{ t('editor.binary') }}</p>
      <el-button size="small" @click="workspace.downloadAt(`${project}/${tab.path}`)">
        {{ t('common.download') }}
      </el-button>
    </div>

    <div v-else class="min-h-0 flex-1">
      <component
        :is="activeComponent"
        ref="inner"
        :key="`${tab.path}:${mode}`"
        :path="tab.path"
        :origin="loadText"
        :original="tab.origin"
        :project="project"
        @dirty="onDirty"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Document } from '@element-plus/icons-vue'
import SourceEditor from './editors/SourceEditor.vue'
import { resolveEditor } from './registry'
import type { EditorApi, EditorContent } from './useEditor'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{ project: string; path: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const inner = ref<(EditorApi & { getText?: () => string }) | null>(null)
const mode = ref<'form' | 'source'>('form')
const saving = ref(false)

/** 进入源码视图时，用它接住表单视图当前的实时内容。 */
const handoff = ref('')

/**
 * 从 store 按路径取当前标签，而不是把对象当 prop 传进来。
 *
 * 传对象时组件持有的是某个时刻的引用；一旦两边指向不同实例，就会出现
 * 「数据已经写进去了但界面不动」。按路径查，永远是当前那一个。
 */
const tab = computed(() => workspace.state(props.project)?.files.find((f) => f.path === props.path))

const def = computed(() => resolveEditor(props.path))

/** 真正加载进编辑器的文本：切到源码视图时用表单视图的实时输出。 */
const loadText = computed(() =>
  mode.value === 'source' && handoff.value ? handoff.value : (tab.value?.origin ?? ''),
)

const activeComponent = computed(() =>
  mode.value === 'form' && def.value.hasForm ? def.value.component : SourceEditor,
)

watch(
  () => props.path,
  () => {
    mode.value = 'form'
    handoff.value = ''
  },
)

watch(mode, async (next, previous) => {
  if (previous === 'form' && next === 'source') {
    // 表单 → 源码：把序列化结果带过去，避免用户刚才的编辑消失
    handoff.value = inner.value?.getContent?.()?.content ?? tab.value?.origin ?? ''
  } else if (previous === 'source' && next === 'form') {
    // 源码 → 表单：把源码内容作为解析输入
    handoff.value = inner.value?.getText?.() ?? tab.value?.origin ?? ''
  }
  await nextTick()
  onDirty(inner.value?.isDirty?.() ?? false)
})

function onDirty(value: boolean) {
  workspace.setDirty(props.project, props.path, value)
}

async function save() {
  const payload: EditorContent | null = inner.value?.getContent?.() ?? null
  if (!payload) return
  saving.value = true
  try {
    await workspace.saveFile(props.project, props.path, payload.content)
    handoff.value = ''
    onDirty(false)
    if (mode.value === 'source' && def.value.hasForm) mode.value = 'form'
    ElMessage.success(t('editor.saved', { path: props.path }))
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    saving.value = false
  }
}

async function revert() {
  handoff.value = ''
  await workspace.reloadFile(props.project, props.path)
  await nextTick()
  onDirty(false)
  ElMessage.info(t('editor.revert'))
}
</script>
