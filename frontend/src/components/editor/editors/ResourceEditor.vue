<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-3xl space-y-3">
      <div v-if="model.error" class="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
        {{ t('editor.readFailed') }}: {{ model.error }}
      </div>

      <p class="rounded-md bg-slate-50 px-3 py-2 text-xs leading-relaxed text-slate-600">
        {{ t('resource.hint') }}
      </p>

      <!-- Docker 项目：原生项目不显示（文件里已经有内容时才显示，避免藏掉已有数据） -->
      <section v-if="showDocker" class="rounded-lg border border-slate-200">
        <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5">
          <el-icon class="text-slate-400"><Box /></el-icon>
          <span class="text-xs font-medium text-slate-700">{{ t('resource.docker') }}</span>
          <span class="flex-1" />
          <el-button text size="small" :icon="Plus" @click="model.root['docker-project'].projects.push({ name: '', path: '' })" />
        </header>
        <div class="space-y-1 p-2">
          <div
            v-for="(item, index) in model.root['docker-project'].projects"
            :key="index"
            class="flex items-center gap-2"
          >
            <el-input v-model="item.name" size="small" class="w-48" :placeholder="t('resource.dockerName')" />
            <el-input v-model="item.path" size="small" class="w-40" :placeholder="t('resource.dockerPath')" />
            <el-button text size="small" :icon="Delete" @click="model.root['docker-project'].projects.splice(index, 1)" />
          </div>
          <p v-if="!model.root['docker-project'].projects.length" class="px-1 py-1 text-xs text-slate-400">
            {{ t('resource.empty') }}
          </p>
        </div>
      </section>

      <!-- 共享目录 -->
      <section class="rounded-lg border border-slate-200">
        <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5">
          <el-icon class="text-slate-400"><FolderOpened /></el-icon>
          <span class="text-xs font-medium text-slate-700">{{ t('resource.shares') }}</span>
          <span class="flex-1" />
          <el-button text size="small" :icon="Plus" @click="model.root['data-share'].shares.push(newShare())" />
        </header>
        <div class="space-y-2 p-2">
          <div
            v-for="(share, index) in model.root['data-share'].shares"
            :key="index"
            class="rounded-md bg-slate-50 p-2"
          >
            <div class="flex items-center gap-2">
              <el-input v-model="share.name" size="small" class="w-56" :placeholder="t('resource.shareName')" />
              <el-button text size="small" :icon="Delete" @click="model.root['data-share'].shares.splice(index, 1)" />
            </div>
            <div class="mt-2 grid grid-cols-[5rem_1fr] items-center gap-2">
              <span class="text-right text-[11px] text-slate-500">{{ t('resource.rw') }}</span>
              <el-select v-model="share.permission.rw" size="small" multiple filterable allow-create default-first-option>
                <el-option v-for="item in share.permission.rw" :key="item" :label="item" :value="item" />
              </el-select>
              <span class="text-right text-[11px] text-slate-500">{{ t('resource.ro') }}</span>
              <el-select v-model="share.permission.ro" size="small" multiple filterable allow-create default-first-option>
                <el-option v-for="item in share.permission.ro" :key="item" :label="item" :value="item" />
              </el-select>
            </div>
          </div>
          <p v-if="!model.root['data-share'].shares.length" class="px-1 py-1 text-xs text-slate-400">
            {{ t('resource.empty') }}
          </p>
        </div>
      </section>

      <!-- 系统链接 -->
      <section class="rounded-lg border border-slate-200">
        <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5">
          <el-icon class="text-slate-400"><Link /></el-icon>
          <span class="text-xs font-medium text-slate-700">{{ t('resource.linker') }}</span>
        </header>
        <div class="space-y-2 p-2">
          <div
            v-for="group in (['bin', 'lib', 'etc'] as const)"
            :key="group"
            class="grid grid-cols-[10rem_1fr] items-center gap-2"
          >
            <span class="text-right text-[11px] text-slate-500">{{ t(`resource.linker${group[0]!.toUpperCase()}${group.slice(1)}`) }}</span>
            <el-select
              v-model="model.root['usr-local-linker'][group]"
              size="small"
              multiple
              filterable
              allow-create
              default-first-option
              placeholder="bin/myapp-cli"
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

      <!-- 飞牛 API 调用列表（api-scope） -->
      <section class="rounded-lg border border-slate-200">
        <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-1.5">
          <el-icon class="text-slate-400"><Connection /></el-icon>
          <span class="text-xs font-medium text-slate-700">{{ t('resource.apiScope') }}</span>
          <span class="flex-1" />
          <span class="mono text-[10px] text-slate-400">api-scope</span>
        </header>
        <div class="p-2">
          <el-checkbox-group v-model="model.apiScope">
            <div v-for="item in apiItems" :key="item.id" class="py-1">
              <el-checkbox :value="item.id">
                <span class="text-xs text-slate-700">{{ item.title }}</span>
                <span class="mono ml-2 text-[10px] text-slate-400">{{ item.id }}</span>
              </el-checkbox>
              <div v-if="item.desc" class="ml-6 text-[11px] leading-relaxed text-slate-500">
                {{ item.desc }}
              </div>
            </div>
          </el-checkbox-group>
          <p v-if="!apiItems.length" class="px-1 py-1 text-xs text-slate-400">
            {{ t('resource.empty') }}
          </p>
          <p class="mt-1 px-1 text-[11px] leading-relaxed text-slate-500">
            {{ t('resource.apiScopeHint') }}
          </p>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Box, Connection, Delete, FolderOpened, Link, Plus } from '@element-plus/icons-vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useProjectsStore } from '@/stores/projects'
import { parseJSON, stringifyOrdered, useEditor, type EditorProps } from '../useEditor'

interface Share {
  name: string
  permission: { rw: string[]; ro: string[] }
}

interface ResourceRoot {
  'docker-project': { projects: { name: string; path: string }[] }
  'data-share': { shares: Share[] }
  'usr-local-linker': { bin: string[]; lib: string[]; etc: string[] }
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

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()
const projects = useProjectsStore()

const newShare = (): Share => ({ name: '', permission: { rw: [], ro: [] } })

const { model, api } = useEditor<Model>(
  props,
  emit,
  (text) => {
    const parsed = parseJSON<Record<string, unknown>>(text, {})
    const raw = (parsed.value ?? {}) as Record<string, unknown>

    const docker = (raw['docker-project'] ?? {}) as { projects?: unknown }
    const share = (raw['data-share'] ?? {}) as { shares?: unknown }
    const linker = (raw['usr-local-linker'] ?? {}) as Record<string, unknown>
    const scope = raw['api-scope']

    const root: ResourceRoot = {
      'docker-project': {
        projects: Array.isArray(docker.projects)
          ? (docker.projects as { name?: string; path?: string }[]).map((p) => ({
              name: String(p.name ?? ''),
              path: String(p.path ?? ''),
            }))
          : [],
      },
      'data-share': {
        shares: Array.isArray(share.shares)
          ? (share.shares as Record<string, unknown>[]).map((s) => {
              const permission = (s.permission ?? {}) as Record<string, unknown>
              return {
                name: String(s.name ?? ''),
                permission: {
                  rw: Array.isArray(permission.rw) ? (permission.rw as string[]) : [],
                  ro: Array.isArray(permission.ro) ? (permission.ro as string[]) : [],
                },
              }
            })
          : [],
      },
      'usr-local-linker': {
        bin: Array.isArray(linker.bin) ? (linker.bin as string[]) : [],
        lib: Array.isArray(linker.lib) ? (linker.lib as string[]) : [],
        etc: Array.isArray(linker.etc) ? (linker.etc as string[]) : [],
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

    const projects = root['docker-project'].projects.filter((p) => p.name.trim() !== '')
    if (projects.length) {
      out['docker-project'] = {
        projects: projects.map((p) => ({ name: p.name, path: p.path || 'docker' })),
      }
    }

    const shares = root['data-share'].shares.filter((s) => s.name.trim() !== '')
    if (shares.length) {
      out['data-share'] = {
        shares: shares.map((s) => {
          const item: Record<string, unknown> = { name: s.name }
          const rw = s.permission.rw.filter(Boolean)
          const ro = s.permission.ro.filter(Boolean)
          if (rw.length || ro.length) {
            item.permission = {}
            if (rw.length) (item.permission as Record<string, unknown>).rw = rw
            if (ro.length) (item.permission as Record<string, unknown>).ro = ro
          }
          return item
        }),
      }
    }

    const linker: Record<string, string[]> = {}
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

/** 勾选项：已知的 5 个 API，外加文件里出现的未知 id（保住它们，别在保存时丢掉）。 */
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
 * 是否显示「Docker 项目」。
 *
 * 原生项目的 resource 里没有 docker-project，显示出来只会让人以为要填；
 * 但文件里已经有内容时不藏——藏了就没法改，等于丢数据。
 */
const showDocker = computed(() => {
  if (model.value.root['docker-project'].projects.length) return true
  return projects.find(props.project ?? '')?.docker === true
})

defineExpose(api)
</script>
