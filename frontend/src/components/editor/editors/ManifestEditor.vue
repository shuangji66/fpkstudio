<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-3xl">
      <div
        v-if="platformWarning"
        class="mb-3 flex items-start gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs leading-relaxed text-amber-800"
      >
        <el-icon class="mt-0.5 shrink-0"><WarningFilled /></el-icon>
        <span>{{ platformWarning }}</span>
      </div>

      <div
        v-for="field in fields"
        :key="field.key"
        class="grid grid-cols-[10rem_1fr] items-start gap-x-3 py-1.5"
      >
        <div class="flex items-center justify-end gap-1 pt-1">
          <span class="text-right text-xs text-slate-600">{{ t(`fields.${field.key}`) }}</span>
          <el-tooltip v-if="HAS_HELP.has(field.key)" placement="top" :show-after="300">
            <template #content>
              <div class="max-w-xs text-xs leading-relaxed">{{ t(`fieldsHelp.${field.key}`) }}</div>
            </template>
            <el-icon class="text-slate-400"><QuestionFilled /></el-icon>
          </el-tooltip>
        </div>

        <div class="min-w-0">
          <el-input
            v-if="field.type === 'input'"
            v-model="model.form[field.key]"
            size="small"
            :placeholder="field.placeholder"
          />
          <el-input
            v-else-if="field.type === 'textarea'"
            v-model="model.form[field.key]"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 8 }"
            resize="none"
          />
          <el-switch v-else-if="field.type === 'switch'" v-model="model.bools[field.key]" />
          <el-select
            v-else-if="field.type === 'select'"
            v-model="model.form[field.key]"
            size="small"
            class="w-full"
          >
            <el-option
              v-for="option in field.options"
              :key="option.value"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
          <el-select
            v-else-if="field.type === 'list'"
            v-model="model.lists[field.key]"
            size="small"
            multiple
            filterable
            allow-create
            default-first-option
            collapse-tags
            :reserve-keyword="false"
            class="w-full"
            placeholder="nodejs_v22"
          >
            <el-option
              v-for="item in model.lists[field.key] ?? []"
              :key="item"
              :label="item"
              :value="item"
            />
          </el-select>
        </div>
      </div>

      <details class="mt-4 rounded-md border border-slate-200 px-3 py-2">
        <summary class="cursor-pointer select-none text-xs text-slate-500">
          其他键（{{ otherKeys.length }}）
        </summary>
        <ul class="mono mt-1 space-y-0.5 text-[11px] text-slate-500">
          <li v-for="key in otherKeys" :key="key">{{ key }} = {{ model.manifest.get(key) }}</li>
          <li v-if="!otherKeys.length">无</li>
        </ul>
      </details>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { QuestionFilled, WarningFilled } from '@element-plus/icons-vue'
import { Manifest } from '@/utils/manifest'
import { useWorkspaceStore } from '@/stores/workspace'
import { useEditor, type EditorProps } from '../useEditor'

type FieldType = 'input' | 'textarea' | 'switch' | 'select' | 'list'

interface FieldDef {
  key: string
  type: FieldType
  placeholder?: string
  options?: { label: string; value: string }[]
}

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()
const workspace = useWorkspaceStore()

const HAS_HELP = new Set([
  'appname',
  'platform',
  'desc',
  'ctl_stop',
  'install_type',
  'install_dep_apps',
  'desktop_uidir',
  'desktop_applaunchname',
  'service_port',
  'changelog',
])

const BOOL_DEFAULT: Record<string, boolean> = {
  ctl_stop: true,
  checkport: true,
  disable_authorization_path: false,
}
/** 值为空时仍然保留的键（其余空值会被删掉，等于「不声明」）。 */
const KEEP_WHEN_EMPTY = ['appname', 'version', 'display_name', 'platform', 'source', 'desktop_uidir']

const fields: FieldDef[] = [
  { key: 'appname', type: 'input' },
  { key: 'version', type: 'input', placeholder: '0.0.1' },
  { key: 'display_name', type: 'input' },
  { key: 'desc', type: 'textarea' },
  {
    key: 'platform',
    type: 'select',
    options: [
      { label: 'all', value: 'all' },
      { label: 'x86', value: 'x86' },
      { label: 'arm', value: 'arm' },
      { label: 'loongarch', value: 'loongarch' },
      { label: 'risc-v', value: 'risc-v' },
    ],
  },
  { key: 'source', type: 'select', options: [{ label: 'thirdparty', value: 'thirdparty' }] },
  { key: 'maintainer', type: 'input' },
  { key: 'maintainer_url', type: 'input' },
  { key: 'distributor', type: 'input' },
  { key: 'distributor_url', type: 'input' },
  { key: 'os_min_version', type: 'input', placeholder: '1.1.3100' },
  { key: 'os_max_version', type: 'input' },
  { key: 'ctl_stop', type: 'switch' },
  {
    key: 'install_type',
    type: 'select',
    options: [
      { label: '（用户选择）', value: '' },
      { label: 'root（系统分区）', value: 'root' },
    ],
  },
  { key: 'install_dep_apps', type: 'list' },
  { key: 'desktop_uidir', type: 'input', placeholder: 'ui' },
  { key: 'desktop_applaunchname', type: 'input' },
  { key: 'service_port', type: 'input' },
  { key: 'checkport', type: 'switch' },
  { key: 'disable_authorization_path', type: 'switch' },
  { key: 'changelog', type: 'input' },
]

interface Model {
  manifest: Manifest
  form: Record<string, string>
  bools: Record<string, boolean>
  lists: Record<string, string[]>
}

const { model } = useEditor<Model>(
  props,
  emit,
  (text) => {
    const manifest = Manifest.parse(text)
    const form: Record<string, string> = {}
    const bools: Record<string, boolean> = {}
    const lists: Record<string, string[]> = {}

    for (const field of fields) {
      const raw = manifest.get(field.key)
      if (field.type === 'switch') {
        bools[field.key] = manifest.has(field.key) ? raw === 'true' : (BOOL_DEFAULT[field.key] ?? false)
      } else if (field.type === 'list') {
        lists[field.key] = raw ? raw.split(':').filter(Boolean) : []
      } else {
        form[field.key] = raw
      }
    }
    return { manifest, form, bools, lists }
  },
  (value) => {
    for (const field of fields) {
      if (field.type === 'switch') {
        value.manifest.set(field.key, value.bools[field.key] ? 'true' : 'false')
      } else if (field.type === 'list') {
        value.manifest.set(field.key, (value.lists[field.key] ?? []).join(':'))
      } else {
        value.manifest.set(field.key, (value.form[field.key] ?? '').trim())
      }
    }
    value.manifest.compact(KEEP_WHEN_EMPTY)
    return `${value.manifest.toText().replace(/\s+$/, '')}\n`
  },
)

/**
 * platform=all 但项目里已经放了架构目录时提醒一下。
 *
 * 这是 fpk 最常见的翻车点：包里带了 x86 二进制却声明 all，
 * 应用中心会把它装到 ARM 设备上然后起不来。
 */
const platformWarning = computed(() => {
  if (model.value.form.platform !== 'all') return ''
  const project = props.project ?? ''
  if (!project) return ''
  const platforms = workspace.state(project)?.platforms ?? []
  return platforms.length ? t('fieldsHelp.platform') : ''
})

const otherKeys = computed(() =>
  model.value.manifest.keys().filter((key) => !fields.some((f) => f.key === key) && key !== 'arch'),
)
</script>
