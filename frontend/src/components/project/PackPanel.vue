<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-4xl space-y-3">
      <div v-if="state.error" class="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
        {{ state.error }}
      </div>

      <!-- 架构选择 -->
      <section class="rounded-lg border border-slate-200">
        <header class="flex items-center gap-2 border-b border-slate-200 px-3 py-2">
          <span class="text-xs font-medium text-slate-700">{{ t('pack.multi') }}</span>
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
                <el-checkbox v-for="name in state.platforms" :key="name" :value="name" :label="name" />
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

          <p v-else class="text-[11px] leading-relaxed text-slate-500">{{ t('pack.singleHint') }}</p>

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
            <el-button
              size="small"
              :icon="Tickets"
              @click="logs.panelOpen = true"
            >
              {{ t('log.title') }}
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
          <li v-for="item in state.results" :key="item.platform" class="flex items-center gap-2 px-3 py-2">
            <el-tag size="small" :type="item.ok ? 'success' : 'danger'" effect="plain">
              {{ item.ok ? t('pack.success') : t('pack.failed') }}
            </el-tag>
            <span class="mono text-xs text-slate-700">{{ item.platform || '—' }}</span>
            <span class="mono min-w-0 flex-1 truncate text-[11px] text-slate-500" :title="item.path">
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
          <li v-for="item in state.artifacts" :key="item.path" class="flex items-center gap-2 px-3 py-2">
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Box, Download, Refresh, Tickets } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useLogsStore } from '@/stores/logs'
import { useSystemStore } from '@/stores/system'
import { downloadEntry } from '@/api/endpoints'
import type { Artifact } from '@/api/types'
import { humanSize } from '@/utils/format'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const logs = useLogsStore()
const system = useSystemStore()

const state = computed(() => workspace.state(props.project))
const outputDir = computed(() => system.info?.outputDir ?? '')
const multi = ref(state.value.settings.multiPlatform)
const selectedPlatforms = ref<string[]>([...state.value.settings.platforms])
const serverDir = ref(state.value.settings.serverDir)
const copyOut = ref(state.value.settings.copyToOutput)

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
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}

onMounted(reload)
</script>
