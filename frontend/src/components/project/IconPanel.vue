<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-4">
    <div class="mx-auto max-w-3xl space-y-3">
      <div class="flex flex-wrap items-start justify-center gap-6">
        <!--
          取景框和输出参数（缩放 / 尺寸 / 圆角 / SVG 底色）在 IconMaker 里：
          首页的「图标」弹窗也用同一份，两处「框里看到的就是导出的方图」必须是一套换算。
        -->
        <IconMaker ref="maker" v-model:size="workSize" :default-sources="defaultSources" />

        <!-- 保存目标 -->
        <div class="min-w-[19rem] flex-1 space-y-2">
          <!-- 生成按钮挪到「写入到」这一行右对齐：它生成的就是下面这个列表里的东西 -->
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-slate-500 dark:text-slate-400">{{ t('iconTab.targets') }}</span>
            <span class="flex-1" />
            <span v-if="progress.total" class="mono text-sm text-slate-500 dark:text-slate-400">
              {{ progress.done }}/{{ progress.total }}
            </span>
            <el-button
              type="primary"
              size="default"
              :icon="Upload"
              :loading="saving"
              @click="save"
            >
              {{ saving ? t('iconTab.saving') : t('iconTab.save') }}
            </el-button>
          </div>

          <!-- 固定的四个位置 -->
          <IconTargetRow
            v-for="target in baseTargets"
            :key="target.key"
            v-model:checked="chosen[target.key]"
            :label-key="target.labelKey"
            :path="target.path"
            :size="targetSize(target)"
            :src="iconURL(target)"
          />

          <!--
            副入口图标：一个项目可以有多个入口，除默认入口（icon_256 / icon_64）之外
            的入口各自一套大 / 小图标，文件名以前缀区分（niubi → niubi_256.png）。
            导入的多入口项目里已经存在的那些，进页面时自动列出来；没写过的尺寸留空，
            勾上「生成」即可补全。
          -->
          <div
            v-for="entry in entryGroups"
            :key="entry.group.id"
            class="space-y-2 rounded-md border border-dashed border-slate-300 dark:border-slate-600 p-2"
          >
            <div class="flex items-center gap-2">
              <span class="shrink-0 text-sm text-slate-600 dark:text-slate-300">{{ t('iconTab.entryPrefix') }}</span>
              <el-input
                v-model="entry.group.prefix"
                size="default"
                class="w-40"
                :maxlength="ICON_PREFIX_MAX"
                :placeholder="t('iconTab.entryPrefixPlaceholder')"
              />
              <span class="flex-1" />
              <el-button
                text
                type="danger"
                size="small"
                :icon="Delete"
                :title="t('iconTab.removeEntry')"
                :aria-label="t('iconTab.removeEntry')"
                @click="removeGroup(entry.group)"
              />
            </div>

            <template v-if="entry.targets.length">
              <IconTargetRow
                v-for="target in entry.targets"
                :key="target.key"
                v-model:checked="chosen[target.key]"
                :label-key="target.labelKey"
                :path="target.path"
                :size="targetSize(target)"
                :src="iconURL(target)"
              />
            </template>
            <p v-else class="text-sm leading-relaxed text-slate-400 dark:text-slate-500">
              {{ t('iconTab.entryPrefixHint') }}
            </p>
          </div>

          <el-button v-if="state.ui" size="default" :icon="Plus" @click="addGroup">
            {{ t('iconTab.addEntry') }}
          </el-button>

          <!-- 小图缩放：标题一行、四个比例一行 -->
          <div class="flex flex-col gap-1 pt-1">
            <span class="text-sm text-slate-600 dark:text-slate-300">{{ t('iconTab.smallScale') }}</span>
            <el-radio-group v-model="smallScale" size="default" :disabled="!hasSmall">
              <el-radio-button :value="1">{{ t('iconTab.scaleSame') }}</el-radio-button>
              <el-radio-button :value="0.5">1/2</el-radio-button>
              <el-radio-button :value="0.25">1/4</el-radio-button>
              <el-radio-button :value="0.125">1/8</el-radio-button>
            </el-radio-group>
          </div>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Upload } from '@element-plus/icons-vue'
import { rawURL } from '@/api/client'
import { listDir } from '@/api/endpoints'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  entryIconPath,
  entryIconPrefixes,
  ENTRY_ICON_DIR,
  ICON_PREFIX_MAX,
  IconError,
  normalizeIconPrefix,
  rasterToPngFile,
} from '@/utils/icon'
import IconMaker from './IconMaker.vue'
import IconTargetRow from './IconTargetRow.vue'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const state = computed(() => workspace.state(props.project))

/**
 * 把错误翻成人话。
 *
 * `utils/icon.ts` 是纯逻辑（不引 vue-i18n），失败时给的是 i18n 键；其余错误照旧用 message。
 */
function errText(e: unknown): string {
  if (e instanceof IconError) return t(e.key)
  return e instanceof Error ? e.message : String(e)
}

/** 进页面默认载入的底图：项目里已有的应用大图标。 */
const DEFAULT_ICON = 'ICON_256.PNG'

interface TargetDef {
  /**
   * 稳定标识。基础图标就是路径，副入口图标是「组 id + 大 / 小」——
   * 前缀边打字边变，路径跟着变，勾选状态得挂在一个不变的东西上。
   */
  key: string
  /** 相对项目根的路径。 */
  path: string
  /** 落地目录，相对项目根；空串表示项目根。 */
  dir: string
  labelKey: string
  small: boolean
  /** 入口图标只存在于带桌面入口的项目里。 */
  needsUi: boolean
}

/**
 * 四个固定的图标位置，取值和 fnpack 打包时用的一致：
 * 应用图标在包根，入口图标在 app/ui/images/，各是大图 256 + 小图 64。
 */
const TARGET_DEFS: TargetDef[] = [
  {
    key: 'ICON_256.PNG',
    path: 'ICON_256.PNG',
    dir: '',
    labelKey: 'iconTab.appLarge',
    small: false,
    needsUi: false,
  },
  {
    key: 'ICON.PNG',
    path: 'ICON.PNG',
    dir: '',
    labelKey: 'iconTab.appSmall',
    small: true,
    needsUi: false,
  },
  {
    key: 'app/ui/images/icon_256.png',
    path: 'app/ui/images/icon_256.png',
    dir: ENTRY_ICON_DIR,
    labelKey: 'iconTab.entryLarge',
    small: false,
    needsUi: true,
  },
  {
    key: 'app/ui/images/icon_64.png',
    path: 'app/ui/images/icon_64.png',
    dir: ENTRY_ICON_DIR,
    labelKey: 'iconTab.entrySmall',
    small: true,
    needsUi: true,
  },
]

/** 一个副入口图标组：前缀决定文件名（niubi → niubi_256.png / niubi_64.png）。 */
interface EntryGroup {
  id: number
  prefix: string
}

const groups = ref<EntryGroup[]>([])
let nextGroupId = 1

/** 一组副入口里的一个目标；前缀还没填时返回 null（界面上只留前缀输入框）。 */
function groupTarget(group: EntryGroup, small: boolean): TargetDef | null {
  const side = small ? 64 : 256
  const path = entryIconPath(group.prefix, side)
  if (!path) return null
  return {
    key: `${group.id}:${small ? 'small' : 'large'}`,
    path,
    dir: ENTRY_ICON_DIR,
    labelKey: small ? 'iconTab.entrySmall' : 'iconTab.entryLarge',
    small,
    needsUi: true,
  }
}

/** 勾选状态按 TargetDef.key 记：目标列表跟着项目形态 / 前缀重算时不会丢。 */
const chosen = reactive<Record<string, boolean>>(
  Object.fromEntries(TARGET_DEFS.map((def) => [def.key, true])),
)

/**
 * 项目里已经存在的图标文件（相对项目根的路径）。
 *
 * 两个用处：给每一行配缩略图（没写过的位置留空框），以及认出导入的多入口项目里
 * 已经有的副入口图标——它们原先在列表里完全看不见。
 */
const existing = ref<Set<string>>(new Set())

/** 缩略图地址；这个位置还没写过时返回空串（列表里就是一个空框）。 */
function iconURL(target: TargetDef): string {
  if (!existing.value.has(target.path)) return ''
  return rawURL('/api/fs/download', {
    path: `${props.project}/${target.path}`,
    rev: state.value.assetRev,
  })
}

/** 固定的那几个位置；不带桌面入口的项目没有后两个。 */
const baseTargets = computed(() => TARGET_DEFS.filter((def) => !def.needsUi || state.value.ui))

/** 副入口组连同它这一组的目标（大在前、小在后）。 */
const entryGroups = computed(() =>
  groups.value.map((group) => ({
    group,
    targets: [groupTarget(group, false), groupTarget(group, true)].filter(
      (target): target is TargetDef => target !== null,
    ),
  })),
)

/** 这次可能被写出去的全部目标（副入口的前缀没填时不参与）。 */
const targets = computed(() => [
  ...baseTargets.value,
  ...entryGroups.value.flatMap((entry) => entry.targets),
])

const hasSmall = computed(() => targets.value.some((def) => def.small))

/**
 * 加一组副入口图标。
 *
 * 顺带把所有图标写入勾选清掉：新增一组意味着这次要写的是另一套图标，
 * 沿用上一组的勾选很容易在点保存时把默认入口的图标一起覆盖掉。
 */
function addGroup() {
  for (const key of Object.keys(chosen)) chosen[key] = false
  const group: EntryGroup = { id: nextGroupId, prefix: '' }
  nextGroupId += 1
  chosen[`${group.id}:large`] = false
  chosen[`${group.id}:small`] = false
  groups.value.push(group)
}

/**
 * 删掉一组副入口图标。
 *
 * 先问一声：这一组里可能已经勾了大 / 小两条写入项，删掉之后列表里那份勾选和前缀一起
 * 消失（磁盘上的图标不删，但用户看不出这一点），误点一下的代价是重填一遍前缀。
 */
async function removeGroup(group: EntryGroup) {
  try {
    await ElMessageBox.confirm(
      t('iconTab.removeEntryConfirm', { prefix: group.prefix || '—' }),
      t('common.tip'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
  groups.value = groups.value.filter((item) => item.id !== group.id)
  delete chosen[`${group.id}:large`]
  delete chosen[`${group.id}:small`]
}

/**
 * 扫一遍现有的图标：项目根的应用图标（ICON_256.PNG / ICON.PNG），
 * 带桌面入口的项目再看 app/ui/images/ 里的入口图标。
 *
 * 形态（有没有桌面入口）是异步读回来的，所以入口目录只在 `state.ui` 为真时才去看：
 * 不带入口的项目里那个目录根本不存在，探它只会拿回一条
 * 「openat app/ui/images: no such file or directory」，把日志和项目状态弄脏。
 *
 * 返回入口目录里的文件名，交给 adoptEntries 认副入口。
 */
async function scanIcons(): Promise<string[]> {
  const paths = new Set<string>()
  const names: string[] = []

  const jobs: Promise<void>[] = [
    listDir(props.project)
      .then((res) => {
        for (const entry of res.entries) if (!entry.dir) paths.add(entry.name)
      })
      .catch(() => undefined),
  ]

  if (state.value.ui) {
    jobs.push(
      listDir(`${props.project}/${ENTRY_ICON_DIR}`)
        .then((res) => {
          for (const entry of res.entries) {
            if (entry.dir) continue
            names.push(entry.name)
            paths.add(`${ENTRY_ICON_DIR}/${entry.name}`)
          }
        })
        .catch(() => undefined),
    )
  }

  await Promise.all(jobs)
  existing.value = paths
  return names
}

/**
 * 把目录里已有的副入口图标补成一组一组的目标。
 *
 * 这些组默认不勾选：它们已经写好了，进来只是想改一改或者补一个尺寸，
 * 不该因为「点一下生成并写入」就把所有入口的图标一起覆盖掉。
 * 界面上已经有的前缀（用户自己加过的）不重复添加。
 */
function adoptEntries(fileNames: string[]) {
  const known = new Set(groups.value.map((group) => normalizeIconPrefix(group.prefix)))
  for (const prefix of entryIconPrefixes(fileNames)) {
    if (known.has(prefix)) continue
    const group: EntryGroup = { id: nextGroupId, prefix }
    nextGroupId += 1
    chosen[`${group.id}:large`] = false
    chosen[`${group.id}:small`] = false
    groups.value.push(group)
  }
}

// ---- 状态 ----

/** 取景框组件（取景 / 缩放 / 尺寸 / 圆角 / SVG 底色都在里面）。 */
const maker = ref<InstanceType<typeof IconMaker> | null>(null)

const saving = ref(false)
const progress = reactive({ done: 0, total: 0 })

/** 工作尺寸（取景框输出方图的边长）：大小图标都按它换算。 */
const workSize = ref(256)
/**
 * 小图缩放。
 *
 * 大图标（应用 / 入口的 `*_256.png`）跟着「尺寸」走；小图标默认「不变」（1）——
 * 这里的「不变」是**和大图标同尺寸**，不是大图的 1/4。想小一些就选 1/2、1/4、1/8，
 * 比例乘的是同一个工作尺寸。以前大图写死 256、小图跟着工作尺寸，改「尺寸」只会把
 * 小图放大，真正想输出 512 的应用大图标反而不动。
 */
const smallScale = ref(1)

function targetSize(target: TargetDef): number {
  if (target.small) return Math.max(1, Math.round(workSize.value * smallScale.value))
  return workSize.value
}

/**
 * 默认底图：先看包根的 ICON_256.PNG，带桌面入口的项目再退回入口目录里的
 * icon_256.png（多入口的项目常常只写过入口图标）。都没有就保持空白取景框，
 * 等用户导入。「按顺序试、失败换下一个」在 IconMaker 里。
 */
const defaultSources = computed(() => {
  const list = [rawURL('/api/fs/download', { path: `${props.project}/${DEFAULT_ICON}` })]
  if (state.value.ui) {
    list.push(rawURL('/api/fs/download', { path: `${props.project}/${ENTRY_ICON_DIR}/icon_256.png` }))
  }
  return list
})

/**
 * 进页面列出已有的副入口图标——导入的多入口项目全靠这一步才看得见它们。
 *
 * 跟着 `state.ui` 走是必要的：不带入口的项目里 app/ui/images 根本不存在，
 * 探测它只会拿回一条 no such file，把日志和项目状态弄脏。
 */
watch(
  () => state.value.ui,
  async (ui) => {
    const names = await scanIcons()
    if (ui) adoptEntries(names)
  },
  { immediate: true },
)

// ---- 输出 ----

async function save() {
  const working = maker.value?.renderWorking()
  if (!working) {
    ElMessage.warning(t('iconTab.needImage'))
    return
  }
  const list = targets.value.filter((target) => chosen[target.key])
  if (!list.length) {
    ElMessage.warning(t('iconTab.needTarget'))
    return
  }

  saving.value = true
  progress.done = 0
  progress.total = list.length
  try {
    for (const target of list) {
      const name = target.path.split('/').pop()!
      const file = await rasterToPngFile(working, targetSize(target), name)
      const dir = target.dir ? `${props.project}/${target.dir}` : props.project
      await workspace.uploadTo(props.project, dir, [file], true)
      progress.done += 1
    }
    // 刚写出去的文件要立刻反映到列表上：缩略图出来，空框不再留空。
    await scanIcons()
    ElMessage.success(t('iconTab.saved', { count: progress.total }))
  } catch (e) {
    ElMessage.error(errText(e))
  } finally {
    saving.value = false
    progress.total = 0
  }
}
</script>
