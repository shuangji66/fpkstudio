<template>
  <div
    class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3"
    :class="{ 'form-stack': layout.isMobile }"
  >
    <div class="mx-auto max-w-3xl">
      <div
        v-if="platformWarning"
        class="mb-3 flex items-start gap-2 rounded-md bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-xs leading-relaxed text-amber-800 dark:text-amber-200"
      >
        <el-icon class="mt-0.5 shrink-0"><WarningFilled /></el-icon>
        <span>{{ platformWarning }}</span>
      </div>

      <div
        v-for="field in fields"
        :key="field.key"
        class="form-row grid grid-cols-[10rem_1fr] items-start gap-x-3 py-1.5"
        :class="field.type === 'switch' ? 'form-row-inline' : ''"
      >
        <div class="form-label flex items-center justify-end gap-1 pt-1">
          <span class="text-right text-xs text-slate-600 dark:text-slate-300">{{ t(`fields.${field.key}`) }}</span>
          <el-tooltip v-if="HAS_HELP.has(field.key)" placement="top" :show-after="300">
            <template #content>
              <div class="max-w-xs text-xs leading-relaxed">{{ t(`fieldsHelp.${field.key}`) }}</div>
            </template>
            <el-icon class="text-slate-400 dark:text-slate-500"><QuestionFilled /></el-icon>
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
            :autosize="{ minRows: field.minRows ?? 2, maxRows: 8 }"
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
          <TagSelect
            v-else-if="field.type === 'list'"
            v-model="model.lists[field.key]"
            :options="listOptions(field)"
            placeholder="nodejs_v22"
            :popper-class="field.presets ? 'dep-apps-popper' : ''"
            class="w-full"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { QuestionFilled, WarningFilled } from '@element-plus/icons-vue'
import { Manifest } from '@/utils/manifest'
import { useLayoutStore } from '@/stores/layout'
import { useWorkspaceStore } from '@/stores/workspace'
import { useEditor, type EditorProps } from '../useEditor'
import TagSelect from '../TagSelect.vue'

type FieldType = 'input' | 'textarea' | 'switch' | 'select' | 'list'

interface FieldDef {
  key: string
  type: FieldType
  placeholder?: string
  options?: { label: string; value: string }[]
  /** switch 关闭时删掉这个键，而不是写 false（例如 micro_app）。 */
  removeWhenOff?: boolean
  /** textarea 的起始行数（默认 2）；链接这类一行就够的字段给 1。 */
  minRows?: number
  /** list 的下拉候选（预选值）；给空数组等于没有候选，只能手输。 */
  presets?: readonly string[]
}

/**
 * `install_dep_apps` 的预选依赖：官方应用中心里能装的那批，一行一个。
 *
 * 只是下拉里的候选，**不限制手输**——`el-select` 仍然开着 allow-create + filterable，
 * 新依赖（尤其是排在名单发布之后的版本号）照样能直接敲进去。
 */
const DEP_APP_PRESETS = [
  'nodejs_v14',
  'nodejs_v16',
  'nodejs_v18',
  'nodejs_v20',
  'nodejs_v22',
  'nodejs_v24',
  'bunjs',
  'java-11-openjdk',
  'java-17-openjdk',
  'java-21-openjdk',
  'python38',
  'python39',
  'python310',
  'python311',
  'python312',
  'go-1.24',
  'go-1.26',
  'erlang',
  'redis',
  'rabbitmq',
  'minio',
  'techfunway-sqlite-manage',
  'prometheus.prometheus',
  'prometheus.node_exporter',
  'grafana.loki',
  'grafana.alloy',
  'nvidia-container-toolkit',
  'AMD-Container-Toolkit',
  'Nvidia-Driver-580',
  'amd.rocm-7.2',
  'i915-sriov-dkms_driver',
  'binder_linux_driver',
  'ite-it87_driver',
  'fn-aic8800',
  'fn-broadcom-sta',
  'fn-open-vm-tools',
  'fn-qemu-ga',
  'OpenVINO-OVMS',
  'ai_installer',
  'FastFlowLM',
  'trim.ai-runtime-amd-migraphx',
  'trim.ai-runtime-amd-vitisai',
] as const

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()
const workspace = useWorkspaceStore()
const layout = useLayoutStore()

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
/**
 * 「安装位置 = 存储位置」在界面上的哨兵值。
 *
 * 它在 manifest 里的真正写法是「不声明 install_type」，所以读写模型时都要和空字符串
 * 互转（见下面的 load / dump）——绝不能让它漏进用户的 manifest。
 */
const INSTALL_STORAGE = '__storage__'

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
  // 链接和更新说明都可能很长，单行输入框会把它藏起来——和 desc 一样用会自己长高的多行框。
  { key: 'maintainer_url', type: 'textarea', minRows: 1 },
  { key: 'distributor', type: 'input' },
  { key: 'distributor_url', type: 'textarea', minRows: 1 },
  { key: 'os_min_version', type: 'input', placeholder: '1.1.3100' },
  { key: 'os_max_version', type: 'input' },
  { key: 'ctl_stop', type: 'switch' },
  {
    key: 'install_type',
    type: 'select',
    // 存储位置就是「留空」：清空的值会被 compact 从 manifest 里删掉，等于不声明这个键；
    // 系统分区写入 install_type=root。这里用一个哨兵值而不是 ''——el-select 对空字符串
    // 会显示占位符（看着像没选），哨兵值才真的把「存储位置」选中；读写模型时翻回去。
    options: [
      { label: t('fields.installTypeStorage'), value: INSTALL_STORAGE },
      { label: t('fields.installTypeRoot'), value: 'root' },
    ],
  },
  { key: 'install_dep_apps', type: 'list', presets: DEP_APP_PRESETS },
  { key: 'desktop_uidir', type: 'input', placeholder: 'ui' },
  { key: 'desktop_applaunchname', type: 'input' },
  { key: 'service_port', type: 'input' },
  { key: 'checkport', type: 'switch' },
  { key: 'disable_authorization_path', type: 'switch' },
  { key: 'micro_app', type: 'switch', removeWhenOff: true },
  { key: 'changelog', type: 'textarea' },
]

interface Model {
  manifest: Manifest
  form: Record<string, string>
  bools: Record<string, boolean>
  lists: Record<string, string[]>
}

const { model, api } = useEditor<Model>(
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
        // 没声明 install_type = 装到存储位置，界面上要显示成选中而不是空
        form[field.key] = field.key === 'install_type' && !raw ? INSTALL_STORAGE : raw
      }
    }
    return { manifest, form, bools, lists }
  },
  (value) => {
    for (const field of fields) {
      if (field.type === 'switch') {
        // removeWhenOff：关闭 = 不声明。写 micro_app = false 和删掉它语义不同，
        // fnOS 只认「有这个键且为 true」，所以关掉时直接把键去掉。
        if (field.removeWhenOff && !value.bools[field.key]) {
          value.manifest.remove(field.key)
        } else {
          value.manifest.set(field.key, value.bools[field.key] ? 'true' : 'false')
        }
      } else if (field.type === 'list') {
        value.manifest.set(field.key, (value.lists[field.key] ?? []).join(':'))
      } else {
        const raw = (value.form[field.key] ?? '').trim()
        value.manifest.set(
          field.key,
          field.key === 'install_type' && raw === INSTALL_STORAGE ? '' : raw,
        )
      }
    }
    value.manifest.compact(KEEP_WHEN_EMPTY)
    return `${value.manifest.toText().replace(/\s+$/, '')}\n`
  },
)

/**
 * 下拉里的候选项 = 预选值 + 已经选中的自定义值（预选里有的不重复列）。
 *
 * 加上后者是为了让当前这份 manifest 里的值在下拉里也能看到（带「已选」状态）；
 * 只列预选值的话，自己敲进去的标识在候选里是「不存在」的。
 */
function listOptions(field: FieldDef): string[] {
  const selected = model.value.lists[field.key] ?? []
  const presets = field.presets ?? []
  return [...presets, ...selected.filter((item) => !presets.includes(item))]
}

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

/**
 * 保存前的拦截：多行值（`"""`）没闭合时不许存。
 *
 * 这是「解析降级了却没报错」的那一类——`Manifest.parse` 对残缺文件不会失败，它把后面的
 * 键吞进多行值里，所以脏判据和 `checkFormat` 都看不出问题。不许存之后，用户能在源码
 * 视图里把引号补上（那时校验就通过了）。
 */
defineExpose({
  ...api,
  saveBlocked: () =>
    model.value.manifest.unterminated.length
      ? t('editor.formatManifestMultiline', {
          keys: model.value.manifest.unterminated.join(', '),
        })
      : '',
})
</script>
