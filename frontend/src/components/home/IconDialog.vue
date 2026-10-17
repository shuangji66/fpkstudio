<template>
  <el-dialog
    v-model="visible"
    :title="t('iconTab.title')"
    class="app-dialog app-dialog-auto"
    modal-class="app-modal"
    :width="layout.isMobile ? '95%' : '30rem'"
    align-center
    lock-scroll
    append-to-body
  >
    <!--
      首页没有项目，这个弹窗只做「取景 → 导出 PNG」：图标制作本身和项目里的
      「图标」页共用 IconMaker，两边的取景换算才不会各写一套。

      `pt-1` 是必须的：`.el-dialog__body` 现在是 `overflow-y: auto` 且没有上内边距，
      取景框紧贴 body 上沿时，那圈 1px 的虚线（画在元素边框之外）会被 body 裁掉，
      上边整整一条看不见。项目图标页的容器自带 `py-4`，所以不用补。
    -->
    <IconMaker ref="maker" class="pt-1" @change="onChange" />

    <!--
      底部只留「导出图片」：关窗有标题栏那个 × 和遮罩 / Esc，不用再摆一个「关闭」——
      它和 × 做的是同一件事，还占掉一行视觉重量。
    -->
    <template #footer>
      <el-button
        size="default"
        type="primary"
        :icon="Download"
        :disabled="!hasImage"
        @click="exportImage"
      >
        {{ t('iconTab.exportPng') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Download } from '@element-plus/icons-vue'
import IconMaker from '@/components/project/IconMaker.vue'
import { useLayoutStore } from '@/stores/layout'

/** 首页工具条上的「图标」弹窗（和设置一样用 v-model 控制显隐）。 */
const visible = defineModel<boolean>({ required: true })

const { t } = useI18n()
const layout = useLayoutStore()

const maker = ref<InstanceType<typeof IconMaker> | null>(null)
/** 没选图时导出按钮置灰（IconMaker 选到图 / 换图时通知）。 */
const hasImage = ref(false)

function onChange(state: { hasImage: boolean }) {
  hasImage.value = state.hasImage
}

/**
 * 导出到本地。
 *
 * 走 object URL + `<a download>`：生成的是浏览器里的 canvas，没有后端路径可下载。
 * 撤销放在下一轮事件循环，和 `api/client.ts` 的下载同一套——点完立刻 revoke
 * 会让部分浏览器来不及取内容。
 */
async function exportImage() {
  const file = await maker.value?.exportPng()
  if (!file) return
  const url = URL.createObjectURL(file)
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = file.name
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
  } finally {
    window.setTimeout(() => URL.revokeObjectURL(url), 10_000)
  }
}
</script>
