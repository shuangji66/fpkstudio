<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div
      v-if="state.error"
      class="mx-auto mb-3 max-w-6xl rounded-md bg-red-50 px-3 py-2 text-xs text-red-700"
    >
      {{ state.error }}
    </div>

    <div
      class="mx-auto grid max-w-6xl grid-cols-1 items-start gap-3 lg:grid-cols-[minmax(0,1fr)_24rem]"
    >
      <!-- 左列：发布设置 / 结果 / 产物 -->
      <div class="space-y-3">
        <!-- 架构选择 -->
        <section class="rounded-lg border border-slate-200">
          <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-2">
            <span class="text-xs font-medium text-slate-700">{{ t('pack.title') }}</span>
            <span class="mx-1 h-3 w-px bg-slate-200" />
            <span class="text-xs text-slate-600">{{ t('pack.multi') }}</span>
            <el-switch v-model="multi" size="small" />
            <span class="flex-1" />
            <el-button text size="small" :icon="Refresh" @click="reload" />
          </header>

          <div class="space-y-3 p-3">
            <template v-if="multi">
              <p class="text-[11px] leading-relaxed text-slate-500">{{ t('pack.multiHint') }}</p>

              <div v-if="state.platforms.length" class="space-y-1">
                <div class="label-text">{{ t('pack.platforms') }}</div>
                <el-checkbox-group v-model="selectedPlatforms">
                  <el-checkbox
                    v-for="name in state.platforms"
                    :key="name"
                    :value="name"
                    :label="name"
                  />
                </el-checkbox-group>
              </div>

              <div
                v-else
                class="rounded-md bg-amber-50 px-3 py-2 text-[11px] leading-relaxed text-amber-800"
              >
                <div>{{ t('pack.noPlatforms') }}</div>
                <div class="mt-1">{{ t('pack.noPlatformsHint') }}</div>
              </div>

              <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3">
                <span class="text-right text-xs text-slate-600">{{ t('pack.serverDir') }}</span>
                <div>
                  <el-input v-model="serverDir" size="small" class="w-64" />
                  <p class="mt-1 text-[11px] text-slate-500">{{ t('pack.serverDirHint') }}</p>
                </div>
              </div>
            </template>

            <p v-else class="text-[11px] leading-relaxed text-slate-500">
              {{ t('pack.singleHint') }}
            </p>

            <div class="flex items-center gap-4">
              <el-checkbox v-model="copyOut">{{ t('pack.copyOut') }}</el-checkbox>
              <span class="mono truncate text-[11px] text-slate-400" :title="outputDir">
                {{ t('pack.outputDir') }}: {{ outputDir }}
              </span>
            </div>

            <div class="flex items-center gap-2">
              <el-button
                type="primary"
                size="small"
                :loading="state.packing"
                :icon="Box"
                @click="run"
              >
                {{ state.packing ? t('pack.packing') : t('pack.start') }}
              </el-button>
            </div>
          </div>
        </section>

        <!-- 结果 -->
        <section v-if="state.results.length" class="rounded-lg border border-slate-200">
          <header class="border-b border-slate-200 px-3 py-2 text-xs font-medium text-slate-700">
            {{ t('pack.resultTitle') }}
          </header>
          <ul class="divide-y divide-slate-100">
            <li
              v-for="item in state.results"
              :key="item.platform"
              class="flex items-center gap-2 px-3 py-2"
            >
              <el-tag size="small" :type="item.ok ? 'success' : 'danger'" effect="plain">
                {{ item.ok ? t('pack.success') : t('pack.failed') }}
              </el-tag>
              <span class="mono text-xs text-slate-700">{{ item.platform || '—' }}</span>
              <span
                class="mono min-w-0 flex-1 truncate text-[11px] text-slate-500"
                :title="item.path"
              >
                {{ item.file || item.log || t('pack.failedTip') }}
              </span>
              <span class="shrink-0 text-[11px] text-slate-400">{{ humanSize(item.size) }}</span>
            </li>
          </ul>
        </section>

        <!-- 产物 -->
        <section class="rounded-lg border border-slate-200">
          <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-2">
            <span class="text-xs font-medium text-slate-700">{{ t('pack.artifacts') }}</span>
            <span class="flex-1" />
            <el-button text size="small" :icon="Refresh" @click="reload" />
          </header>
          <ul v-if="state.artifacts.length" class="divide-y divide-slate-100">
            <li
              v-for="item in state.artifacts"
              :key="item.path"
              class="flex items-center gap-2 px-3 py-2"
            >
              <el-tag size="small" effect="plain" type="info">
                {{ item.scope === 'output' ? t('pack.scopeOutput') : t('pack.scopeProject') }}
              </el-tag>
              <span class="mono min-w-0 flex-1 truncate text-xs text-slate-700" :title="item.path">
                {{ item.name }}
              </span>
              <span class="shrink-0 text-[11px] text-slate-400">{{ item.sizeRaw }}</span>
              <span class="shrink-0 text-[11px] text-slate-400">{{ item.modTime }}</span>
              <el-button
                text
                size="small"
                :icon="Download"
                @click="downloadEntry(getDownloadPath(item))"
              />
            </li>
          </ul>
          <p v-else class="px-3 py-3 text-xs text-slate-400">{{ t('pack.noArtifacts') }}</p>
        </section>
      </div>

      <!-- 右列：manifest 预览 -->
      <aside class="lg:sticky lg:top-0">
        <section class="rounded-lg border border-slate-200">
          <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-2">
            <span class="text-xs font-medium text-slate-700">{{ t('overview.manifest') }}</span>
            <span class="flex-1" />
            <el-button
              text
              size="small"
              :icon="Refresh"
              :title="t('common.refresh')"
              @click="loadManifest"
            />
            <el-button
              text
              size="small"
              :icon="EditPen"
              :title="t('overview.editManifest')"
              @click="editManifest"
            />
          </header>

          <p
            v-if="manifestError"
            class="px-3 py-3 text-[11px] leading-relaxed break-words text-amber-700"
          >
            {{ manifestError }}
          </p>

          <div v-else-if="manifestRows.length" class="px-3 py-2">
            <div class="mono text-[11px] leading-[1.55]">
              <div
                v-for="row in manifestRows"
                :key="row.key"
                class="flex"
                :class="clamped(row) ? 'cursor-pointer' : ''"
                @click="toggle(row)"
              >
                <span class="shrink-0 whitespace-pre text-slate-400">{{ row.padded }} = </span>
                <span
                  class="min-w-0 flex-1 break-words whitespace-pre-wrap text-slate-800"
                  :class="clamped(row) ? 'line-clamp-3' : ''"
                  >{{ row.value || '—' }}</span
                >
              </div>
            </div>
          </div>

          <p v-else class="px-3 py-3 text-xs text-slate-400">{{ t('overview.noManifest') }}</p>
        </section>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Box, Download, EditPen, Refresh } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useSystemStore } from '@/stores/system'
import { downloadEntry, readFile } from '@/api/endpoints'
import type { Artifact } from '@/api/types'
import { humanSize } from '@/utils/format'
import { Manifest } from '@/utils/manifest'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const system = useSystemStore()

const state = computed(() => workspace.state(props.project))
const outputDir = computed(() => system.info?.outputDir ?? '')
const multi = ref(state.value.settings.multiPlatform)
const selectedPlatforms = ref<string[]>([...state.value.settings.platforms])
const serverDir = ref(state.value.settings.serverDir)
const copyOut = ref(state.value.settings.copyToOutput)

// ---- manifest 预览 ----

const manifestError = ref('')
const manifestRows = ref<PreviewRow[]>([])
const expanded = reactive(new Set<string>())

/** 逐行取原文，但排除 checksum：它每次打包都会变，看它没有意义。 */
const EXCLUDED_KEYS = new Set(['checksum'])
/** 超过这个长度就折成三行，点一下展开——desc 常常是几百字。 */
const CLAMP_OVER = 90

interface PreviewRow {
  key: string
  value: string
  /** 键按最长的一个右补空格，配 monospace 就是对齐的 `key = value`。 */
  padded: string
}

async function loadManifest() {
  manifestError.value = ''
  try {
    const res = await readFile(`${props.project}/manifest`)
    const entries = Manifest.parse(res.content).entries.filter(
      (e) => e.kind === 'kv' && e.key && !EXCLUDED_KEYS.has(e.key),
    )
    const width = entries.reduce((max, e) => Math.max(max, e.key.length), 0)
    manifestRows.value = entries.map((e) => ({
      key: e.key,
      value: e.value,
      padded: e.key.padEnd(width),
    }))
    expanded.clear()
  } catch (e) {
    manifestRows.value = []
    manifestError.value = t('overview.manifestFailed', {
      error: e instanceof Error ? e.message : String(e),
    })
  }
}

/** 长值折起来了吗（折起来的行才有「点一下展开」）。 */
function clamped(row: PreviewRow): boolean {
  return row.value.length > CLAMP_OVER && !expanded.has(row.key)
}

function toggle(row: PreviewRow) {
  if (expanded.has(row.key)) expanded.delete(row.key)
  else if (clamped(row)) expanded.add(row.key)
}

function editManifest() {
  void workspace.openFile(props.project, `${props.project}/manifest`)
}

/**
 * manifest 标签打开时 / 保存后跟着刷新预览。
 *
 * 盯的是磁盘基线（tab.origin），所以敲键盘不会每次都去读文件，但保存、还原、
 * 刚打开这几种情况都能立刻同步。
 */
const manifestBaseline = computed(
  () => state.value.files.find((f) => f.path === 'manifest')?.origin ?? '',
)
watch(manifestBaseline, () => {
  void loadManifest()
})

/** 产物在项目目录里时用相对路径下载，在输出目录里时后端只能给绝对路径，改用项目内路径。 */
function getDownloadPath(item: Artifact): string {
  return item.scope === 'output' ? `${props.project}/${item.name}` : item.path
}

async function persist() {
  await workspace.updateSettings(props.project, {
    multiPlatform: multi.value,
    platforms: selectedPlatforms.value,
    serverDir: serverDir.value,
    copyToOutput: copyOut.value,
  })
}

async function reload() {
  await workspace.reloadPackageInfo(props.project)
  multi.value = state.value.settings.multiPlatform
  selectedPlatforms.value = [...state.value.settings.platforms]
  serverDir.value = state.value.settings.serverDir
  copyOut.value = state.value.settings.copyToOutput
  await loadManifest()
}

async function run() {
  if (multi.value && !selectedPlatforms.value.length) {
    ElMessage.warning(t('pack.selectPlatform'))
    return
  }
  await persist()
  try {
    const results = await workspace.pack(props.project)
    const failed = results.filter((r) => !r.ok)
    if (failed.length) ElMessage.error(t('pack.failedTip'))
    else ElMessage.success(t('pack.resultTitle'))
    // 打包不改项目里的 manifest（在暂存副本上改），但顺手重读一次预览，
    // 免得预览停在打包前的样子。
    await loadManifest()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

onMounted(reload)
</script>
