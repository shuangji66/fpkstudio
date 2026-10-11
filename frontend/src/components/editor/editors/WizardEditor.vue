<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-3xl space-y-3">
      <div v-if="model.error" class="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
        {{ t('editor.readFailed') }}: {{ model.error }}
      </div>

      <div class="flex items-center gap-2">
        <span class="text-xs text-slate-500">{{ t('wizard.file', { name: props.path }) }}</span>
        <span class="flex-1" />
        <el-button size="small" :icon="Plus" @click="addStep">{{ t('wizard.stepTitle') }}</el-button>
      </div>

      <p class="rounded-md bg-slate-50 px-3 py-2 text-xs leading-relaxed text-slate-600">
        {{ t('wizard.hint') }}
      </p>

      <p v-if="!model.steps.length" class="py-6 text-center text-xs text-slate-400">
        {{ t('wizard.empty') }}
      </p>

      <section
        v-for="(step, stepIndex) in model.steps"
        :key="stepIndex"
        class="rounded-lg border border-slate-200"
      >
        <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5">
          <span class="text-xs font-medium text-slate-700">#{{ stepIndex + 1 }}</span>
          <el-input
            v-model="step.stepTitle"
            size="small"
            class="w-72"
            :placeholder="t('wizard.stepTitle')"
          />
          <span class="flex-1" />
          <el-button text size="small" :icon="Plus" @click="addItem(step)" />
          <el-button text size="small" :icon="Delete" @click="model.steps.splice(stepIndex, 1)" />
        </header>

        <div class="space-y-2 p-2">
          <div
            v-for="(item, itemIndex) in step.items"
            :key="itemIndex"
            class="rounded-md bg-slate-50 p-2"
          >
            <div class="flex flex-wrap items-center gap-2">
              <el-select v-model="item.type" size="small" class="w-28">
                <el-option :label="t('wizard.types.text')" value="text" />
                <el-option :label="t('wizard.types.tips')" value="tips" />
                <el-option :label="t('wizard.types.radio')" value="radio" />
                <el-option :label="t('wizard.types.switch')" value="switch" />
                <el-option :label="t('wizard.types.checkbox')" value="checkbox" />
              </el-select>
              <el-input
                v-if="item.type !== 'tips'"
                v-model="item.field"
                size="small"
                class="w-44"
                :placeholder="t('wizard.field')"
              />
              <el-input v-model="item.label" size="small" class="w-52" :placeholder="t('wizard.label')" />
              <el-button text size="small" :icon="Delete" @click="step.items.splice(itemIndex, 1)" />
            </div>

            <div class="mt-2 flex flex-wrap items-center gap-2">
              <el-input
                v-model="item.initValue"
                size="small"
                class="w-40"
                :placeholder="t('wizard.initValue')"
              />
              <el-input
                v-model="item.helpText"
                size="small"
                class="min-w-[16rem] flex-1"
                :placeholder="t('wizard.helpText')"
              />
              <el-checkbox v-if="item.type !== 'tips'" v-model="item.required" :label="t('wizard.required')" />
            </div>

            <div
              v-if="item.type === 'radio' || item.type === 'checkbox'"
              class="mt-2 space-y-1 border-t border-slate-200 pt-2"
            >
              <div class="flex items-center gap-2">
                <span class="text-[11px] text-slate-500">{{ t('wizard.options') }}</span>
                <el-button text size="small" :icon="Plus" @click="item.options.push({ label: '', value: '' })" />
              </div>
              <div
                v-for="(option, optionIndex) in item.options"
                :key="optionIndex"
                class="flex items-center gap-2"
              >
                <el-input v-model="option.label" size="small" class="w-44" :placeholder="t('wizard.optionLabel')" />
                <el-input v-model="option.value" size="small" class="w-44" :placeholder="t('wizard.optionValue')" />
                <el-button text size="small" :icon="Delete" @click="item.options.splice(optionIndex, 1)" />
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Delete, Plus } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { stringifyOrdered, useEditor, type EditorProps } from '../useEditor'

interface WizardOption {
  label: string
  value: string
}

interface WizardItem {
  type: string
  field: string
  label: string
  initValue: string
  helpText: string
  required: boolean
  requiredMessage: string
  options: WizardOption[]
}

interface WizardStep {
  stepTitle: string
  items: WizardItem[]
}

interface Model {
  steps: WizardStep[]
  error: string
}

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()

function emptyItem(type = 'text'): WizardItem {
  return {
    type,
    field: '',
    label: '',
    initValue: '',
    helpText: '',
    required: false,
    requiredMessage: '',
    options: [],
  }
}

const { model } = useEditor<Model>(
  props,
  emit,
  (text) => {
    const trimmed = text.trim()
    if (!trimmed || trimmed === '[]') return { steps: [], error: '' }
    try {
      const raw = JSON.parse(trimmed) as unknown
      if (!Array.isArray(raw)) return { steps: [], error: '顶层不是数组' }
      const steps: WizardStep[] = raw.map((step) => {
        const source = (step ?? {}) as Record<string, unknown>
        const items = Array.isArray(source.items) ? (source.items as Record<string, unknown>[]) : []
        return {
          stepTitle: String(source.stepTitle ?? ''),
          items: items.map((item) => {
            const rules = Array.isArray(item.rules) ? (item.rules as Record<string, unknown>[]) : []
            const first = rules[0] ?? {}
            return {
              type: String(item.type ?? 'text'),
              field: String(item.field ?? ''),
              label: String(item.label ?? ''),
              initValue: item.initValue === undefined ? '' : String(item.initValue),
              helpText: String(item.helpText ?? ''),
              required: first.required === true,
              requiredMessage: String(first.message ?? ''),
              options: Array.isArray(item.options)
                ? (item.options as Record<string, unknown>[]).map((o) => ({
                    label: String(o.label ?? ''),
                    value: String(o.value ?? ''),
                  }))
                : [],
            }
          }),
        }
      })
      return { steps, error: '' }
    } catch (e) {
      return { steps: [], error: e instanceof Error ? e.message : String(e) }
    }
  },
  (value) => {
    const steps = value.steps.map((step) => {
      const items: Record<string, unknown>[] = step.items.map((item) => {
        const out: Record<string, unknown> = { type: item.type }
        // tips 类型只有 helpText，没有 field / label / rules
        if (item.type === 'tips') {
          if (item.helpText) out.helpText = item.helpText
          return out
        }
        if (item.field) out.field = item.field
        if (item.label) out.label = item.label
        if (item.type !== 'switch' && item.initValue !== '') out.initValue = item.initValue
        if (item.type === 'switch') out.initValue = item.initValue === 'true'
        if (item.helpText) out.helpText = item.helpText
        if (item.options.length) {
          out.options = item.options.map((o) => ({ label: o.label, value: o.value }))
        }
        if (item.required) out.rules = [{ required: true, message: item.requiredMessage || item.label }]
        return out
      })
      const out: Record<string, unknown> = {}
      if (step.stepTitle) out.stepTitle = step.stepTitle
      if (items.length) out.items = items
      return out
    })
    return stringifyOrdered(steps)
  },
)

function addStep() {
  model.value.steps.push({ stepTitle: '', items: [emptyItem()] })
}

function addItem(step: WizardStep) {
  step.items.push(emptyItem())
}
</script>
