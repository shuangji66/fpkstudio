<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-4xl space-y-3">
      <div v-if="model.error" class="rounded-md bg-red-50 dark:bg-red-950/40 px-3 py-2 text-sm text-red-700 dark:text-red-300">
        {{ t('editor.readFailed') }}: {{ model.error }}
      </div>

      <!-- 文件名不在这儿重复：文件标签上已经写着了 -->
      <div class="flex items-center justify-end">
        <el-button size="default" :icon="Plus" @click="addStep">{{ t('wizard.addStep') }}</el-button>
      </div>

      <p v-if="!model.steps.length" class="py-6 text-center text-sm text-slate-400 dark:text-slate-500">
        {{ t('wizard.empty') }}
      </p>

      <section
        v-for="(step, stepIndex) in model.steps"
        :key="stepIndex"
        class="rounded-lg border border-slate-200 dark:border-slate-700"
      >
        <header class="flex items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-3 py-1.5">
          <!-- 序号 + 步骤名算一组，整体占一半宽度；右边留给操作按钮 -->
          <div class="flex w-1/2 min-w-0 items-center gap-2">
            <span class="shrink-0 text-sm font-medium text-slate-700 dark:text-slate-200">#{{ stepIndex + 1 }}</span>
            <el-input
              v-model="step.stepTitle"
              size="default"
              class="flex-1"
              :placeholder="t('wizard.stepTitle')"
            />
          </div>
          <span class="flex-1" />
          <el-button
            text
            size="default"
            :icon="Plus"
            :title="t('wizard.addField')"
            :aria-label="t('wizard.addField')"
            @click="addItem(step)"
          />
          <el-button
            text
            type="danger"
            size="default"
            :icon="Delete"
            :title="t('wizard.removeStep')"
            :aria-label="t('wizard.removeStep')"
            @click="removeStep(stepIndex)"
          />
        </header>

        <div class="space-y-2 p-2">
          <div
            v-for="(item, itemIndex) in step.items"
            :key="itemIndex"
            class="rounded-md bg-slate-50 dark:bg-slate-900 p-2"
          >
            <!--
              顶部一行：左边「字段序号 + 添加校验」占一半宽度，右边是顺序 / 删除三个图标按钮
              （靠右对齐）。三个图标按钮之间的间距收到 0.5（原来 2），并排时不需要长距离横移。
            -->
            <div class="flex items-center gap-1">
              <div class="flex w-1/2 min-w-0 items-center gap-2">
                <span class="shrink-0 text-sm font-medium text-slate-700 dark:text-slate-200">#{{ itemIndex + 1 }}</span>
                <el-button size="default" :icon="CirclePlus" @click="addRule(item)">
                  {{ t('wizard.addRule') }}
                </el-button>
              </div>
              <span class="flex-1" />
              <!--
                调整这一项在本步骤里的位置。表单项的顺序就是向导里的显示顺序，
                改动直接落在内存模型上（保存时按顺序回写源码）。
              -->
              <div class="flex items-center gap-0.5">
                <el-button
                  text
                  size="default"
                  class="tree-tool"
                  :icon="ArrowUp"
                  :title="t('wizard.moveUp')"
                  :aria-label="t('wizard.moveUp')"
                  :disabled="itemIndex === 0"
                  @click="moveItem(step, itemIndex, -1)"
                />
                <el-button
                  text
                  size="default"
                  class="tree-tool"
                  :icon="ArrowDown"
                  :title="t('wizard.moveDown')"
                  :aria-label="t('wizard.moveDown')"
                  :disabled="itemIndex === step.items.length - 1"
                  @click="moveItem(step, itemIndex, 1)"
                />
                <el-button
                  text
                  type="danger"
                  size="default"
                  class="tree-tool"
                  :icon="Delete"
                  :title="t('wizard.removeField')"
                  :aria-label="t('wizard.removeField')"
                  @click="step.items.splice(itemIndex, 1)"
                />
              </div>
            </div>

            <!-- 字段类型 + 字段标签：各占一半，同一行 -->
            <div class="mt-2 grid grid-cols-2 gap-2">
              <div class="flex min-w-0 flex-col gap-1">
                <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('wizard.fieldType') }}</span>
                <el-select v-model="item.type" size="default" @change="onTypeChange(item)">
                  <el-option
                    v-for="type in TYPES"
                    :key="type.value"
                    :label="t(type.labelKey)"
                    :value="type.value"
                  />
                </el-select>
              </div>
              <div class="flex min-w-0 flex-col gap-1">
                <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('wizard.fieldLabel') }}</span>
                <el-input v-model="item.label" size="default" />
              </div>
            </div>

            <!-- 字段名 + 默认值：各占一半，同一行（提示文字类字段两个都没有） -->
            <div v-if="item.type !== 'tips'" class="mt-2 grid grid-cols-2 gap-2">
              <div class="flex min-w-0 flex-col gap-1">
                <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('wizard.fieldName') }}</span>
                <el-input v-model="item.field" size="default" />
              </div>
              <div class="flex min-w-0 flex-col gap-1">
                <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('wizard.initValue') }}</span>
                <el-input
                  v-if="item.type === 'text' || item.type === 'password'"
                  v-model="item.initValue"
                  size="default"
                  :type="item.type === 'password' ? 'password' : 'text'"
                  :show-password="item.type === 'password'"
                />
                <el-switch
                  v-else-if="item.type === 'switch'"
                  :model-value="item.initValue === 'true'"
                  @change="setSwitch(item, $event)"
                />
                <el-select
                  v-else-if="item.type === 'radio' || item.type === 'select'"
                  v-model="item.initValue"
                  size="default"
                  clearable
                >
                  <el-option
                    v-for="(option, index) in item.options"
                    :key="index"
                    :label="option.label"
                    :value="option.value"
                  />
                </el-select>
                <TagSelect
                  v-else-if="item.type === 'checkbox'"
                  v-model="item.initValues"
                  size="default"
                  :options="item.options"
                  class="w-full"
                />
              </div>
            </div>

            <!-- 选项（单选 / 多选 / 下拉）：夹在默认值框和辅助文本框之间 -->
            <div
              v-if="item.type === 'radio' || item.type === 'checkbox' || item.type === 'select'"
              class="mt-2 space-y-1 border-t border-slate-200 dark:border-slate-700 pt-2"
            >
              <div class="flex items-center gap-2">
                <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('wizard.options') }}</span>
                <el-button
                  text
                  size="default"
                  :icon="Plus"
                  :title="t('wizard.addOption')"
                  :aria-label="t('wizard.addOption')"
                  @click="addOption(item)"
                />
                <span class="text-sm text-slate-400 dark:text-slate-500">{{ t('wizard.optionsHint') }}</span>
              </div>
              <div
                v-for="(option, optionIndex) in item.options"
                :key="optionIndex"
                class="flex items-center gap-2"
              >
                <el-input
                  v-model="option.label"
                  size="default"
                  class="w-44"
                  :placeholder="t('wizard.optionLabel')"
                />
                <el-input
                  v-model="option.value"
                  size="default"
                  class="w-44"
                  :placeholder="t('wizard.optionValue')"
                />
                <el-button
                  text
                  type="danger"
                  size="default"
                  :icon="Delete"
                  :title="t('wizard.removeOption')"
                  :aria-label="t('wizard.removeOption')"
                  @click="item.options.splice(optionIndex, 1)"
                />
              </div>
            </div>

            <!-- 辅助文本：单独一行（说明文字常常是一整段），写长了会自己撑高 -->
            <div class="mt-2 flex flex-col gap-1">
              <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('wizard.helperText') }}</span>
              <el-input
                v-model="item.helpText"
                type="textarea"
                :autosize="{ minRows: 1, maxRows: 6 }"
                resize="none"
              />
            </div>

            <!-- 校验规则：必填 / 范围 / 长度 / 正则，可以叠加多条 -->
            <div v-if="item.rules.length" class="mt-2 space-y-1 border-t border-slate-200 dark:border-slate-700 pt-2">
              <div
                v-for="(rule, ruleIndex) in item.rules"
                :key="ruleIndex"
                class="flex flex-wrap items-center gap-2"
              >
                <el-select
                  v-model="rule.type"
                  size="default"
                  class="w-28"
                  @change="onRuleChange(rule)"
                >
                  <el-option
                    v-for="type in RULE_TYPES"
                    :key="type.value"
                    :label="t(type.labelKey)"
                    :value="type.value"
                  />
                </el-select>

                <template v-if="rule.type === 'required'">
                  <el-checkbox v-model="rule.required" size="default">
                    {{ t('wizard.required') }}
                  </el-checkbox>
                </template>
                <template v-else-if="rule.type === 'min'">
                  <el-input-number
                    v-model="rule.min"
                    size="default"
                    controls-position="right"
                    class="w-24"
                  />
                  <span class="text-sm text-slate-400 dark:text-slate-500">-</span>
                  <el-input-number
                    v-model="rule.max"
                    size="default"
                    controls-position="right"
                    class="w-24"
                  />
                </template>
                <template v-else-if="rule.type === 'len'">
                  <el-input-number
                    v-model="rule.len"
                    size="default"
                    controls-position="right"
                    class="w-24"
                  />
                </template>
                <template v-else>
                  <el-input
                    v-model="rule.pattern"
                    size="default"
                    class="w-64"
                    :placeholder="t('wizard.patternPlaceholder')"
                  />
                </template>

                <el-input
                  v-model="rule.message"
                  size="default"
                  class="min-w-[12rem] flex-1"
                  :placeholder="t('wizard.validateMessage')"
                />
                <el-button
                  text
                  type="danger"
                  size="default"
                  :icon="Delete"
                  :title="t('wizard.removeRule')"
                  :aria-label="t('wizard.removeRule')"
                  @click="item.rules.splice(ruleIndex, 1)"
                />
              </div>
            </div>

            <!-- 这里还不认识的键（fnOS 以后加的字段）原样保留，不在表单里呈现 -->
            <p v-if="Object.keys(item.extra).length" class="mt-2 text-sm text-slate-400 dark:text-slate-500">
              {{ t('wizard.keptKeys', { keys: Object.keys(item.extra).join(', ') }) }}
            </p>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ArrowDown, ArrowUp, CirclePlus, Delete, Plus } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { useEditor, type EditorProps } from '../useEditor'
import TagSelect from '../TagSelect.vue'
import {
  WIZARD_RULE_TYPES,
  WIZARD_TYPES,
  dumpWizard,
  newWizardItem,
  newWizardRule,
  newWizardStep,
  nextWizardField,
  parseWizard,
  type WizardItem,
  type WizardModel,
  type WizardRule,
  type WizardStep,
} from '@/utils/wizard'

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()

/** 字段类型 / 校验类型在界面上显示什么名字由这里决定（值就是写进 JSON 的键）。 */
const TYPES = WIZARD_TYPES.map((value) => ({ value, labelKey: `wizard.types.${value}` }))
const RULE_TYPES = WIZARD_RULE_TYPES.map((value) => ({ value, labelKey: ruleLabelKey(value) }))

function ruleLabelKey(type: string): string {
  return type === 'min' ? 'wizard.range' : type === 'len' ? 'wizard.length' : `wizard.${type}`
}

/**
 * 新建规则时填一组能直接用的默认值 + 提示文案，省得每项都手打。
 *
 * 数值与正则的默认值在 utils/wizard.ts 里（跟界面语言无关），提示文案在这里。
 */
function applyRuleDefaults(rule: WizardRule) {
  const key =
    rule.type === 'min'
      ? 'wizard.defaultRangeMessage'
      : rule.type === 'len'
        ? 'wizard.defaultLengthMessage'
        : rule.type === 'pattern'
          ? 'wizard.defaultPatternMessage'
          : 'wizard.defaultRequiredMessage'
  rule.message = t(key)
}

function emptyItem(): WizardItem {
  // 字段名同时是脚本里的变量名，重名会互相覆盖，所以按已有的名字往后排
  return newWizardItem(nextWizardField(model.value), t('wizard.defaultItem'))
}

const { model, api } = useEditor<WizardModel>(
  props,
  emit,
  (text) => parseWizard(text),
  (value) => dumpWizard(value),
)

function addStep() {
  model.value.steps.push(newWizardStep(t('wizard.defaultStep'), emptyItem()))
}

/**
 * 删掉一个步骤。
 *
 * 先确认：一个步骤里可能已经有整屏的表单项，误点一下全没了（这一页没有撤销）。
 * 字段 / 选项 / 校验的删除不确认——它们只丢一行，逐条问会让人不敢动手。
 */
async function removeStep(index: number) {
  const step = model.value.steps[index]
  if (!step) return
  try {
    await ElMessageBox.confirm(
      t('wizard.removeStepConfirm', { index: index + 1, title: step.stepTitle || '—' }),
      t('common.tip'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
  model.value.steps.splice(index, 1)
}

/**
 * 向导文件还没创建时先给一个步骤。
 *
 * 新建项目 / 刚导入的项目里这几份向导往往一个都不存在，打开是一个纯空白的表单，
 * 用户得先点一下「添加步骤」才有东西可填——这里直接铺好第一步（带一个示例字段）。
 *
 * 判据是磁盘基线为空：文件真的不存在、或者是个空文件。已经有内容的不动，
 * 哪怕内容是 `[]`——那是用户自己清空的，不该每次进来又塞一个步骤回去。
 * 只在这里（初次进入）判断，之后用户删光了也不会再自动加。
 */
if (!props.original && !model.value.steps.length) {
  addStep()
}

function addItem(step: WizardStep) {
  step.items.push(emptyItem())
}

/**
 * 把第 index 个表单项往上（delta = -1）或往下（+1）挪一格。
 *
 * 顺序就是源码里的顺序（dumpWizard 按数组顺序写），所以这里只动数组：
 * 越界的调用不会发生（按钮在首 / 末项上是禁用的），真到了边界就原样返回。
 */
function moveItem(step: WizardStep, index: number, delta: number) {
  const next = index + delta
  if (next < 0 || next >= step.items.length) return
  const [item] = step.items.splice(index, 1)
  if (item) step.items.splice(next, 0, item)
}

function addOption(item: WizardItem) {
  const index = item.options.length + 1
  item.options.push({ label: t('wizard.defaultOptionLabel', { index }), value: String(index) })
}

function addRule(item: WizardItem) {
  const rule = newWizardRule('required')
  applyRuleDefaults(rule)
  item.rules.push(rule)
}

/** 换校验类型时按新类型重新填默认提示，避免把上一种类型的文案留下。 */
function onRuleChange(rule: WizardRule) {
  applyRuleDefaults(rule)
}

/** 换字段类型时收拾默认值：多选 / 开关 / 单值不能混着存。 */
function onTypeChange(item: WizardItem) {
  if (item.type === 'checkbox') item.initValues = []
  else if (item.type === 'switch') item.initValue = 'false'
  else item.initValue = ''
  if (item.type === 'tips' && !item.helpText) item.helpText = t('wizard.defaultTips')
}

/** 开关的默认值在 JSON 里是布尔，模型里统一存成 'true' / 'false' 字符串。 */
function setSwitch(item: WizardItem, value: boolean | string | number) {
  item.initValue = value ? 'true' : 'false'
}

defineExpose({
  ...api,
  /**
   * 解析失败时拦住保存。
   *
   * 解析失败 = 表单里那份模型是空的，`getContent()` 交出去的是 `[]`。直接保存就会把
   * 用户的向导文件清空——那是不可逆的，所以宁可不让存：源码视图里改回来再存。
   */
  saveBlocked: () =>
    model.value.error ? t('editor.formatWizard', { detail: model.value.error }) : '',
})
</script>
