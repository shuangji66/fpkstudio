<template>
  <div class="relative flex h-full min-h-0 bg-slate-100 dark:bg-slate-900">
    <aside
      v-if="showTree"
      class="flex shrink-0 flex-col border-r border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800"
      :style="{ width: `${treeWidth}px` }"
    >
      <FileTree :project="name" @resize-start="startResize" />
    </aside>

    <div
      v-if="showTree"
      class="w-1 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-sky-300 dark:hover:bg-sky-700"
      :title="t('editor.resizeHint')"
      @pointerdown="startResize"
    />

    <section class="flex min-w-0 flex-1 flex-col bg-white dark:bg-slate-800">
      <FileTabs :project="name" :mobile="mobile" />

      <div class="relative min-h-0 flex-1">
        <FileBrowser
          v-show="activeTab === FILES_TAB"
          :project="name"
          :mobile="mobile"
          class="absolute inset-0"
        />
        <OverviewPanel
          v-show="activeTab === OVERVIEW_TAB"
          :project="name"
          class="absolute inset-0"
        />
        <ShortcutsPanel
          v-show="activeTab === SHORTCUTS_TAB"
          :project="name"
          class="absolute inset-0"
        />
        <IconPanel v-show="activeTab === ICON_TAB" :project="name" class="absolute inset-0" />

        <!--
          桌面：打开的文件就是子标签，全部保持挂载，切标签不丢未保存的内容。
          移动端换一种呈现（见下面的移动端子页面），但同样是全部挂载。
        -->
        <template v-if="!mobile">
          <EditorHost
            v-for="file in files"
            v-show="activeTab === file.path"
            :key="file.path"
            class="absolute inset-0"
            :project="name"
            :path="file.path"
          />
        </template>
      </div>
    </section>

    <!--
      移动端的文件子页面：盖住整个项目区（含子标签栏），编辑器工具栏最左边那个后退
      等同于关掉这个文件标签——未保存的提醒走的是同一段逻辑（useCloseFile）。
    -->
    <template v-if="mobile">
      <EditorHost
        v-for="file in files"
        v-show="activeTab === file.path"
        :key="file.path"
        class="absolute inset-0 z-10"
        :project="name"
        :path="file.path"
        :back="() => closeFile(name, file.path)"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FileTree from '@/components/project/FileTree.vue'
import FileTabs from '@/components/project/FileTabs.vue'
import FileBrowser from '@/components/project/FileBrowser.vue'
import OverviewPanel from '@/components/project/OverviewPanel.vue'
import ShortcutsPanel from '@/components/project/ShortcutsPanel.vue'
import IconPanel from '@/components/project/IconPanel.vue'
import EditorHost from '@/components/editor/EditorHost.vue'
import { useCloseFile } from '@/components/project/useCloseFile'
import { FILES_TAB, ICON_TAB, OVERVIEW_TAB, SHORTCUTS_TAB, useWorkspaceStore } from '@/stores/workspace'
import { useProjectsStore } from '@/stores/projects'

const props = defineProps<{
  name: string
  /** 移动端形态：固定标签页模式、不显示文件树、文件走子页面。 */
  mobile?: boolean
}>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const projects = useProjectsStore()
const { closeFile } = useCloseFile()

const state = computed(() => workspace.state(props.name))
const files = computed(() => state.value.files)
const activeTab = computed(() => state.value.activeTab)

/** 左栏文件树只在「左栏模式」下占地方；移动端永远没有它。 */
const showTree = computed(() => !props.mobile && state.value.viewMode === 'tree')

/** 左栏能拉到的最小宽度：再窄工具栏就摆不下了。 */
const TREE_MIN = 180

/**
 * 左栏能拉到的最大宽度：**窗口宽度的一半**。
 *
 * 每次拖都现算，而不是写死一个像素值：写死 640 在窄窗口上等于「能把编辑区挤到只剩
 * 两百来像素」，而窗口拉宽之后那个上限又白白浪费——反正右边永远得留一半给编辑区。
 */
function treeMax() {
  return Math.max(TREE_MIN, Math.floor(window.innerWidth / 2))
}

const stored = window.localStorage.getItem('fpkstudio.treeWidth')
// 宽度记在 localStorage 里，换到窄窗口 / 小屏上打开时要按新窗口重新钳一次
const treeWidth = ref(Math.min(stored ? Number(stored) : 300, treeMax()))

function startResize(event: PointerEvent) {
  // 手柄是按钮，不挡一下的话按下会顺手选中/聚焦（触屏上还会触发一次点击）
  event.preventDefault()
  const startX = event.clientX
  const startWidth = treeWidth.value
  const move = (e: PointerEvent) => {
    treeWidth.value = Math.min(Math.max(startWidth + (e.clientX - startX), TREE_MIN), treeMax())
  }
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    // pointercancel 也要摘：触屏手势被系统接管、或者拖拽被取消时不会再来 pointerup，
    // 这两个监听就留在窗口上了——之后鼠标一动树就跟着缩，直到下一次松手。
    window.removeEventListener('pointercancel', up)
    window.localStorage.setItem('fpkstudio.treeWidth', String(treeWidth.value))
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

onMounted(async () => {
  if (!projects.count) await projects.refresh()
  await workspace.ensure(props.name)
})

/**
 * 标签被关掉 = 退出这个项目：把它在内存里的那份状态一起丢掉，下次打开重新读盘
 * （留着它的坏处见 `forget` 的注释——项目在磁盘上，应用外面随时可能被换掉）。
 *
 * 用卸载（而不是 beforeUnmount）：先让子组件（编辑器宿主）把没保存的草稿交回 store，
 * 之后才丢；反过来的话那些回调会往一份刚被丢掉的状态里写。
 *
 * 这里也是「关标签」的唯一收口：标签上的关闭按钮、右键的「关闭其他标签」、项目被重命名
 * / 删除时的关标签，最后都是这个组件被卸载。别改成在某个关闭入口里清——漏掉一个入口的
 * 表现就是「从那条路关掉再打开，看到的还是老内容」。
 */
onUnmounted(() => workspace.forget(props.name))
</script>
