<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-3xl space-y-3">
      <div v-if="model.error" class="rounded-md bg-red-50 dark:bg-red-950/40 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {{ t('editor.readFailed') }}: {{ model.error }}
      </div>

      <p class="rounded-md bg-slate-50 dark:bg-slate-900 px-3 py-2 text-sm leading-relaxed text-slate-600 dark:text-slate-300">
        {{ t('resource.hint') }}
      </p>

      <!-- Docker 项目：原生项目不显示（文件里已经有内容时才显示，避免藏掉已有数据） -->
      <section v-if="showDocker" class="rounded-lg border border-slate-200 dark:border-slate-700">
        <header class="flex items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-3 py-1.5">
          <el-icon class="text-slate-400 dark:text-slate-500"><Box /></el-icon>
          <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('resource.docker') }}</span>
          <span class="flex-1" />
          <el-button
            text
            size="small"
            :icon="Plus"
            @click="model.root['docker-project'].projects.push({ name: '', path: '', extra: {} })"
          />
        </header>
        <div class="space-y-1 p-2">
          <div
            v-for="(item, index) in model.root['docker-project'].projects"
            :key="index"
            class="flex items-center gap-2"
          >
            <el-input v-model="item.name" size="default" class="w-48" :placeholder="t('resource.dockerName')" />
            <el-input v-model="item.path" size="default" class="w-40" :placeholder="t('resource.dockerPath')" />
            <el-button
              text
              type="danger"
              size="small"
              class="ml-auto"
              :icon="Delete"
              :title="t('resource.remove')"
              :aria-label="t('resource.remove')"
              @click="model.root['docker-project'].projects.splice(index, 1)"
            />
          </div>
          <p v-if="!model.root['docker-project'].projects.length" class="px-1 py-1 text-sm text-slate-400 dark:text-slate-500">
            {{ t('resource.empty') }}
          </p>
        </div>
      </section>

      <!-- 共享目录 -->
      <section class="rounded-lg border border-slate-200 dark:border-slate-700">
        <header class="flex items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-3 py-1.5">
          <el-icon class="text-slate-400 dark:text-slate-500"><FolderOpened /></el-icon>
          <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('resource.shares') }}</span>
          <span class="flex-1" />
          <el-button text size="small" :icon="Plus" @click="model.root['data-share'].shares.push(newShare())" />
        </header>
        <div class="space-y-2 p-2">
          <div
            v-for="(share, index) in model.root['data-share'].shares"
            :key="index"
            class="rounded-md bg-slate-50 dark:bg-slate-900 p-2"
          >
            <!-- 路径占半行（另一半留给删除按钮推到最右） -->
            <div class="flex items-center gap-2">
              <el-input v-model="share.name" size="default" class="w-1/2" :placeholder="t('resource.shareName')" />
              <el-button
                text
                type="danger"
                size="small"
                class="ml-auto"
                :icon="Delete"
                :title="t('resource.remove')"
                :aria-label="t('resource.remove')"
                @click="removeShare(index, share.name)"
              />
            </div>
            <!--
              可写 / 只读：标题一行、控件一行，两个一组同一行——实际几乎只会填其中一个，
              各占一整行太浪费高度。
            -->
            <div class="mt-2 grid grid-cols-2 gap-2">
              <div class="flex min-w-0 flex-col gap-1">
                <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('resource.rw') }}</span>
                <el-select
                  v-model="share.permission.rw"
                  size="default"
                  multiple
                  filterable
                  allow-create
                  default-first-option
                >
                  <el-option v-for="item in share.permission.rw" :key="item" :label="item" :value="item" />
                </el-select>
              </div>
              <div class="flex min-w-0 flex-col gap-1">
                <span class="text-sm text-slate-500 dark:text-slate-400">{{ t('resource.ro') }}</span>
                <el-select
                  v-model="share.permission.ro"
                  size="default"
                  multiple
                  filterable
                  allow-create
                  default-first-option
                >
                  <el-option v-for="item in share.permission.ro" :key="item" :label="item" :value="item" />
                </el-select>
              </div>
            </div>
          </div>
          <p v-if="!model.root['data-share'].shares.length" class="px-1 py-1 text-sm text-slate-400 dark:text-slate-500">
            {{ t('resource.empty') }}
          </p>
        </div>
      </section>

      <!-- 系统链接 -->
      <section class="rounded-lg border border-slate-200 dark:border-slate-700">
        <header class="flex items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-3 py-1.5">
          <el-icon class="text-slate-400 dark:text-slate-500"><Link /></el-icon>
          <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('resource.linker') }}</span>
          <span class="text-sm text-slate-400 dark:text-slate-500">{{ t('resource.linkerPath') }}</span>
        </header>
        <div class="space-y-2 p-2">
          <div
            v-for="group in (['bin', 'lib', 'etc'] as const)"
            :key="group"
            class="flex min-w-0 flex-col gap-1"
          >
            <span class="text-sm text-slate-500 dark:text-slate-400">{{ t(`resource.linker${group[0]!.toUpperCase()}${group.slice(1)}`) }}</span>
            <el-select
              v-model="model.root['usr-local-linker'][group]"
              size="default"
              multiple
              filterable
              allow-create
              default-first-option
              :placeholder="LINKER_PLACEHOLDER[group]"
            >
              <el-option
                v-for="item in model.root['usr-local-linker'][group] ?? []"
                :key="item"
                :label="item"
                :value="item"
              />
            </el-select>
          </div>
        </div>
      </section>

      <!-- 飞牛 API 调用列表 -->
      <section class="rounded-lg border border-slate-200 dark:border-slate-700">
        <header class="flex items-center gap-2 border-b border-slate-200 dark:border-slate-700 px-3 py-1.5">
          <el-icon class="text-slate-400 dark:text-slate-500"><Connection /></el-icon>
          <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('resource.apiScope') }}</span>
        </header>
        <div class="p-2">
          <!-- 授权项：标题 + 问号（功能描述挂在问号上），开关推到行尾 -->
          <div v-for="item in apiItems" :key="item.id" class="flex items-center gap-1 py-1">
            <span class="text-sm text-slate-700 dark:text-slate-200">{{ item.title }}</span>
            <el-tooltip v-if="item.desc" placement="top" :show-after="300">
              <template #content>
                <div class="max-w-xs text-xs leading-relaxed">{{ item.desc }}</div>
              </template>
              <el-icon class="text-slate-400 dark:text-slate-500"><QuestionFilled /></el-icon>
            </el-tooltip>
            <span class="flex-1" />
            <el-switch :model-value="model.apiScope.includes(item.id)" @change="setScope(item.id, $event)" />
          </div>
          <p v-if="!apiItems.length" class="px-1 py-1 text-sm text-slate-400 dark:text-slate-500">
            {{ t('resource.empty') }}
          </p>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Box, Connection, Delete, FolderOpened, Link, Plus, QuestionFilled } from '@element-plus/icons-vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { useProjectsStore } from '@/stores/projects'
import { parseJSON, stringifyOrdered, useEditor, type EditorProps } from '../useEditor'

interface ProjectItem {
  name: string
  path: string
  /** 项目里表单不认识的键，保存时原样写回。 */
  extra: Record<string, unknown>
}

interface Share {
  name: string
  permission: {
    rw: string[]
    ro: string[]
    /** permission 里不认识的键（以后加的），保存时原样写回。 */
    extra: Record<string, unknown>
  }
  /** share 上不认识的键，保存时原样写回。 */
  extra: Record<string, unknown>
}

/** 挑出「表单不认识的键」，保存时原样写回。 */
function pickExtra(raw: Record<string, unknown>, known: string[]): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(raw)) {
    if (!known.includes(key)) out[key] = value
  }
  return out
}

interface ResourceRoot {
  'docker-project': { projects: ProjectItem[]; extra: Record<string, unknown> }
  'data-share': { shares: Share[]; extra: Record<string, unknown> }
  'usr-local-linker': { bin: string[]; lib: string[]; etc: string[]; extra: Record<string, unknown> }
  [key: string]: unknown
}

interface Model {
  root: ResourceRoot
  /** config/resource 里的顶层 api-scope 数组。 */
  apiScope: string[]
  error: string
}

/**
 * 已知的飞牛 API 授权项（写进 manifest 之外的 config/resource: {"api-scope": [...]}）。
 *
 * id 是 fnOS 侧的接口名，key 只是 i18n 用的短名——id 里带点，直接当 i18n 键会
 * 被 vue-i18n 当成路径分隔符。
 */
const API_SCOPES = [
  { id: 'trim.file.sharedAccess', key: 'sharedAccess' },
  { id: 'trim.file.userAccess', key: 'userAccess' },
  { id: 'trim.file.userAcl', key: 'userAcl' },
  { id: 'trim.file.path', key: 'path' },
  { id: 'trim.system.getPlatformConfig', key: 'platformConfig' },
] as const

const KNOWN_IDS = new Set<string>(API_SCOPES.map((item) => item.id))

/**
 * 三组系统链接的示例路径（占位提示），写的是**相对 app/** 的路径。
 *
 * 三组各举一个对得上类型的例子：可执行文件、共享库、配置文件——同一个 `bin/myapp-cli`
 * 摆在 lib / etc 下面只会误导。
 */
const LINKER_PLACEHOLDER: Record<'bin' | 'lib' | 'etc', string> = {
  bin: 'bin/myapp-cli',
  lib: 'lib/myapp_lib.so',
  etc: 'etc/myapp.conf',
}

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()
const projects = useProjectsStore()

const newShare = (): Share => ({ name: '', extra: {}, permission: { rw: [], ro: [], extra: {} } })

/**
 * 删掉一条共享目录声明。
 *
 * 先问一声：这一行里可能填了路径和一个读写 / 只读用户列表，误点一下就全没了，
 * 而且列表里没有撤销。
 */
async function removeShare(index: number, name: string) {
  try {
    await ElMessageBox.confirm(
      t('resource.removeShareConfirm', { name: name.trim() || '—' }),
      t('common.tip'),
      { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
  model.value.root['data-share'].shares.splice(index, 1)
}

const { model, api } = useEditor<Model>(
  props,
  emit,
  (text) => {
    const parsed = parseJSON<Record<string, unknown>>(text, {})
    const raw = (parsed.value ?? {}) as Record<string, unknown>

    const docker = (raw['docker-project'] ?? {}) as Record<string, unknown>
    const share = (raw['data-share'] ?? {}) as Record<string, unknown>
    const linker = (raw['usr-local-linker'] ?? {}) as Record<string, unknown>
    const scope = raw['api-scope']

    const root: ResourceRoot = {
      'docker-project': {
        // 已知块 / 每一项的未知键都留着（见 pickExtra 的说明）
        extra: pickExtra(docker, ['projects']),
        projects: Array.isArray(docker.projects)
          ? (docker.projects as Record<string, unknown>[]).map((p) => ({
              name: String(p.name ?? ''),
              path: String(p.path ?? ''),
              extra: pickExtra(p, ['name', 'path']),
            }))
          : [],
      },
      'data-share': {
        extra: pickExtra(share, ['shares']),
        shares: Array.isArray(share.shares)
          ? (share.shares as Record<string, unknown>[]).map((s) => {
              const permission = (s.permission ?? {}) as Record<string, unknown>
              return {
                name: String(s.name ?? ''),
                extra: pickExtra(s, ['name', 'permission']),
                permission: {
                  rw: Array.isArray(permission.rw) ? (permission.rw as string[]) : [],
                  ro: Array.isArray(permission.ro) ? (permission.ro as string[]) : [],
                  extra: pickExtra(permission, ['rw', 'ro']),
                },
              }
            })
          : [],
      },
      'usr-local-linker': {
        bin: Array.isArray(linker.bin) ? (linker.bin as string[]) : [],
        lib: Array.isArray(linker.lib) ? (linker.lib as string[]) : [],
        etc: Array.isArray(linker.etc) ? (linker.etc as string[]) : [],
        extra: pickExtra(linker, ['bin', 'lib', 'etc']),
      },
    }

    // 保留未知的顶层键（api-scope 由 model.apiScope 自己管，跳过）
    for (const [key, value] of Object.entries(raw)) {
      if (key === 'api-scope') continue
      if (!(key in root)) root[key] = value
    }

    return {
      root,
      apiScope: Array.isArray(scope) ? (scope as unknown[]).map(String) : [],
      error: parsed.error,
    }
  },
  (value) => {
    const root = value.root
    const out: Record<string, unknown> = {}

    // 已知块：有内容（有效项或者未知键）就写回，未知键一个都不丢。以前是从固定字段重建
    // 每一个块和每一项，于是「改个 share 名再保存」会把不认识的键一起删掉
    // （`usr-local-linker.sbin`、share / permission 上以后加的键）。
    const putBlock = (
      key: string,
      extra: Record<string, unknown>,
      body: Record<string, unknown>,
    ) => {
      const block = { ...extra, ...body }
      if (Object.keys(block).length) out[key] = block
    }

    const projects = root['docker-project'].projects.filter((p) => p.name.trim() !== '')
    putBlock(
      'docker-project',
      root['docker-project'].extra,
      projects.length
        ? {
            projects: projects.map((p) => ({
              ...p.extra,
              name: p.name,
              path: p.path || 'docker',
            })),
          }
        : {},
    )

    const shares = root['data-share'].shares.filter((s) => s.name.trim() !== '')
    putBlock(
      'data-share',
      root['data-share'].extra,
      shares.length
        ? {
            shares: shares.map((s) => {
              const item: Record<string, unknown> = { ...s.extra, name: s.name }
              const permission: Record<string, unknown> = { ...s.permission.extra }
              const rw = s.permission.rw.filter(Boolean)
              const ro = s.permission.ro.filter(Boolean)
              if (rw.length) permission.rw = rw
              if (ro.length) permission.ro = ro
              if (Object.keys(permission).length) item.permission = permission
              return item
            }),
          }
        : {},
    )

    const linker: Record<string, unknown> = { ...root['usr-local-linker'].extra }
    for (const group of ['bin', 'lib', 'etc'] as const) {
      const list = (root['usr-local-linker'][group] ?? []).filter(Boolean)
      if (list.length) linker[group] = list
    }
    if (Object.keys(linker).length) out['usr-local-linker'] = linker

    const scope = value.apiScope.filter(Boolean)
    if (scope.length) out['api-scope'] = scope

    for (const [key, v] of Object.entries(root)) {
      if (key === 'docker-project' || key === 'data-share' || key === 'usr-local-linker') continue
      if (key === 'api-scope') continue
      out[key] = v
    }
    return stringifyOrdered(out)
  },
)

/** 授权项：已知的 5 个 API，外加文件里出现的未知 id（保住它们，别在保存时丢掉）。 */
const apiItems = computed(() => {
  const known = API_SCOPES.map((item) => ({
    id: item.id,
    title: t(`resource.api.${item.key}.title`),
    desc: t(`resource.api.${item.key}.desc`),
  }))
  const extra = model.value.apiScope
    .filter((id) => !KNOWN_IDS.has(id))
    .map((id) => ({ id, title: id, desc: '' }))
  return [...known, ...extra]
})

/**
 * 开关 = 这个 id 在不在 `api-scope` 里。
 *
 * 勾选状态本身就是那份字符串数组（写回文件的是它），所以开关只做增删：新选的追加到末尾，
 * 保持用户勾选的先后顺序（整份数组按顺序写回文件）。
 */
function setScope(id: string, value: boolean | string | number) {
  const list = model.value.apiScope
  if (!value) {
    model.value.apiScope = list.filter((item) => item !== id)
    return
  }
  if (!list.includes(id)) list.push(id)
}

/**
 * 是否显示「Docker 项目」。
 *
 * 原生项目的 resource 里没有 docker-project，显示出来只会让人以为要填；
 * 但文件里已经有内容时不藏——藏了就没法改，等于丢数据。
 */
const showDocker = computed(() => {
  if (model.value.root['docker-project'].projects.length) return true
  return projects.find(props.project ?? '')?.docker === true
})

defineExpose({
  ...api,
  /**
   * 解析失败时拦住保存。
   *
   * 解析失败 = 表单里那份模型是空的，`getContent()` 交出去的是一份空配置。直接保存
   * 就会把用户那份（也许只是少了个括号、还能救的）文件覆盖掉——不可逆，所以宁可
   * 不让存：在源码视图里改回来再存。
   */
  saveBlocked: () =>
    model.value.error ? t('editor.formatJson', { detail: model.value.error }) : '',
})
</script>
