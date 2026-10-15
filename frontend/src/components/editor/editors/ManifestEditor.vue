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

      <!--
        一行一组字段（分组见 script 里的 rows）：组里每个字段自己「标题一行、控件一行」，
        整行铺满宽度。窄屏下 .form-stack 把每组拆回一行一个，不用在这里写三元。
      -->
      <div
        v-for="row in rows"
        :key="row.fields[0].key"
        class="form-row grid gap-x-3 gap-y-1.5 py-1.5"
        :class="rowClass(row)"
      >
        <div v-for="field in row.fields" :key="field.key" class="flex min-w-0 flex-col gap-1">
          <div class="form-label flex items-center gap-1">
            <span class="text-sm text-slate-600 dark:text-slate-300">{{ t(`fields.${field.key}`) }}</span>
            <el-tooltip v-if="HAS_HELP.has(field.key)" placement="top" :show-after="300">
              <template #content>
                <div class="max-w-xs text-xs leading-relaxed">{{ t(`fieldsHelp.${field.key}`) }}</div>
              </template>
              <el-icon class="text-slate-400 dark:text-slate-500"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>

          <div class="min-w-0">
            <!-- 尺寸一律 size="default"：全局配的是 small（12px 字），这里要 14px -->
            <el-input
              v-if="field.type === 'input'"
              v-model="model.form[field.key]"
              size="default"
              :placeholder="field.placeholder"
            />
            <el-input
              v-else-if="field.type === 'textarea'"
              v-model="model.form[field.key]"
              type="textarea"
              size="default"
              :autosize="{ minRows: field.minRows ?? 2, maxRows: 8 }"
              resize="none"
            />
            <el-switch v-else-if="field.type === 'switch'" v-model="model.bools[field.key]" />
            <el-select
              v-else-if="field.type === 'select'"
              v-model="model.form[field.key]"
              size="default"
              class="w-full"
            >
              <el-option
                v-for="option in field.options"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
            <!--
              默认入口：既能从下拉里挑，也能直接敲。候选是**每次打开下拉时现读**的
              `app/<入口目录>/config` 里的入口 id（见 refreshLaunchOptions）——在入口配置
              里刚加的那个入口，回来填默认入口时就得能选到，而不是停在打开编辑器那一刻的
              快照上。
            -->
            <el-select
              v-else-if="field.type === 'entry'"
              v-model="model.form[field.key]"
              size="default"
              class="w-full"
              filterable
              allow-create
              default-first-option
              :reserve-keyword="false"
              :loading="launchLoading"
              :placeholder="t('fields.entryPlaceholder')"
              @visible-change="onLaunchVisible"
            >
              <el-option v-for="id in launchOptions" :key="id" :label="id" :value="id" />
            </el-select>
            <TagSelect
              v-else-if="field.type === 'list'"
              v-model="model.lists[field.key]"
              :options="listOptions(field)"
              size="default"
              :popper-class="field.presets ? 'dep-apps-popper' : ''"
              class="w-full"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { QuestionFilled, WarningFilled } from '@element-plus/icons-vue'
import { Manifest } from '@/utils/manifest'
import { readFile } from '@/api/endpoints'
import { entryIds } from '@/utils/entries'
import { useLayoutStore } from '@/stores/layout'
import { useWorkspaceStore } from '@/stores/workspace'
import { useEditor, type EditorProps } from '../useEditor'
import TagSelect from '../TagSelect.vue'

type FieldType = 'input' | 'textarea' | 'switch' | 'select' | 'list' | 'entry'

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

/** 桌面上的一行：一组字段 + 这一行的网格类（不给就按字段个数算列数）。 */
interface FieldRow {
  fields: FieldDef[]
  grid?: string
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

/**
 * 值为空时仍然保留的键（其余空值会被删掉，等于「不声明」）。
 *
 * `source` 已经不在表单里了（见 rows），留在这里是为了让 compact 别顺手删掉用户文件里
 * 那条空的 `source`：不显示它，也不动它。
 */
const KEEP_WHEN_EMPTY = ['appname', 'version', 'display_name', 'platform', 'source', 'desktop_uidir']

/**
 * 表单字段，**外层数组就是桌面上的一行**：一行里放几个字段由这一组有几个决定
 * （1 / 2 / 4 列，见 colsClass），组里每个字段自己「标题一行、控件一行」。行可以自己给
 * `grid`（见开发者 / 发布者那一组）。
 *
 * 分组按语义成对：标识 / 显示名称、版本 / 架构、开发者 / 发布者那两对、最低 / 最高系统
 * 版本、安装位置 / 服务端口、入口目录 / 默认入口，依赖应用和四个开关各一行，描述和更新
 * 说明各自一整行（都是会长高的多行框）。窄屏下这层分组会被 .form-stack 拆掉，回到一行
 * 一个，拆开后的顺序就是 DOM 顺序。
 *
 * 这里只有**真正有控件的键**：`source` 不在其中——包都是第三方的，这个值改不了也没
 * 必要改，所以既不显示也不回写（读进来的原值原样留在文件里）。
 */
const rows: FieldRow[] = [
  // 应用标识 + 显示名称
  {
    fields: [
      { key: 'appname', type: 'input' },
      { key: 'display_name', type: 'input' },
    ],
  },
  // 版本号 + 架构
  {
    fields: [
      { key: 'version', type: 'input', placeholder: '0.0.1' },
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
    ],
  },
  /*
   * 开发者 / 发布者与各自的链接：桌面上要「单行框一对、多行框一对」（同一行是同种控件，
   * 宽度和高度才对得上），但 **DOM 顺序按窄屏的竖排写**（开发者 → 开发者链接 → 发布者 →
   * 发布者链接），桌面靠 grid-flow-col + 两行把它折成两列。
   *
   * 反过来的话（DOM 按桌面的行顺序写）窄屏会竖排成「开发者、发布者、开发者链接、
   * 发布者链接」——同一个人的两栏被隔开，读起来是错位的。
   */
  {
    grid: 'grid-cols-2 grid-rows-[auto_auto] grid-flow-col',
    fields: [
      { key: 'maintainer', type: 'input' },
      { key: 'maintainer_url', type: 'textarea', minRows: 1 },
      { key: 'distributor', type: 'input' },
      { key: 'distributor_url', type: 'textarea', minRows: 1 },
    ],
  },
  // 最低 + 最高系统版本
  {
    fields: [
      { key: 'os_min_version', type: 'input' },
      { key: 'os_max_version', type: 'input' },
    ],
  },
  // 安装位置 + 服务端口
  {
    fields: [
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
      { key: 'service_port', type: 'input' },
    ],
  },
  // 入口目录 + 默认入口
  {
    fields: [
      { key: 'desktop_uidir', type: 'input', placeholder: 'ui' },
      // 默认入口的候选来自 `app/<入口目录>/config` 的 `.url` 键，点开下拉时现读一次
      { key: 'desktop_applaunchname', type: 'entry' },
    ],
  },
  // 依赖应用：多选标签要宽度，自己占一整行，紧挨在开关上面
  { fields: [{ key: 'install_dep_apps', type: 'list', presets: DEP_APP_PRESETS }] },
  // 四个开关一行
  {
    fields: [
      { key: 'ctl_stop', type: 'switch' },
      { key: 'checkport', type: 'switch' },
      { key: 'disable_authorization_path', type: 'switch' },
      { key: 'micro_app', type: 'switch', removeWhenOff: true },
    ],
  },
  // 描述
  { fields: [{ key: 'desc', type: 'textarea' }] },
  // 更新说明
  { fields: [{ key: 'changelog', type: 'textarea' }] },
]

/**
 * 打平一份给读写模型用。
 *
 * 顺序 = 表单从上到下、一行从左到右的顺序，所以**新键的追加顺序就是用户看到的顺序**；
 * 已经在文件里的键各自待在原位（`Manifest.set` 只改值不搬行）。
 */
const fields = rows.flatMap((row) => row.fields)

/**
 * 桌面上的网格类：默认按这一组有几个字段算列数（见 colsClass），行可以自己给 `grid`
 * 覆盖（开发者 / 发布者那一组要 grid-flow-col）。
 */
function rowClass(row: FieldRow): string {
  return row.grid ?? colsClass(row.fields)
}

/**
 * 一行里放几个字段 → 网格列数。
 *
 * 不把类名拼成 `grid-cols-${n}`：Tailwind 是扫源码里的**字面量**收集类名的，
 * 拼出来的名字不会进产物，表现是「列数写对了，样式完全没生效」。
 */
function colsClass(fields: FieldDef[]): string {
  if (fields.length >= 4) return 'grid-cols-4'
  if (fields.length === 2) return 'grid-cols-2'
  return 'grid-cols-1'
}

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
 * 「默认入口」下拉的候选 = 配置里的入口 id + 当前值。
 *
 * 当前值也并进去：这份 manifest 里可能填着一个在 config 里已经不存在（或者还没读出来）
 * 的 id，不列出来的话 `el-select` 会把它显示成空的样子，用户一保存就把它弄丢了。
 */
const launchIds = ref<string[]>([])
const launchLoading = ref(false)
const launchOptions = computed(() => {
  const current = (model.value.form.desktop_applaunchname ?? '').trim()
  const ids = launchIds.value
  return current && !ids.includes(current) ? [current, ...ids] : ids
})

/**
 * 现读一遍 `app/<入口目录>/config`，把里面的入口 id 当下拉候选。
 *
 * 读的是**磁盘上的最新内容**（`readFile`，不走 workspace 的编辑器缓存）：用户常常先在
 * 「入口配置」里加一个入口，回来才填默认入口，缓存里那份可能还没保存 / 还是旧的。
 * 读不到（没有 config、路径换了、没权限）就当没有候选——这只是「没得挑」，用户照样能
 * 手输，所以不弹错。
 */
async function refreshLaunchOptions(): Promise<void> {
  const project = props.project
  if (!project) return
  // 入口目录以表单里当前填的为准（默认 ui，见 fieldsHelp.desktop_uidir）
  const uidir = (model.value.form.desktop_uidir ?? '').trim() || 'ui'
  launchLoading.value = true
  try {
    const res = await readFile(`${project}/app/${uidir}/config`)
    launchIds.value = entryIds(res.content)
  } catch {
    launchIds.value = []
  } finally {
    launchLoading.value = false
  }
}

/** 下拉展开时才读（点输入框也算展开）。收起那一下不用再读一次。 */
function onLaunchVisible(visible: boolean): void {
  if (visible) void refreshLaunchOptions()
}

// 挂载时先读一次：不点开也能看到候选里有哪几个入口。
onMounted(() => void refreshLaunchOptions())

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
