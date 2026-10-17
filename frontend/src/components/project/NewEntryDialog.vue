<template>
  <el-dialog
    v-model="visible"
    :title="t('tree.new')"
    class="app-dialog app-dialog-auto"
    modal-class="app-modal"
    :width="layout.isMobile ? '95%' : '26rem'"
    align-center
    lock-scroll
    append-to-body
    @opened="focusInput"
  >
    <el-input
      ref="inputEl"
      v-model="name"
      size="default"
      :placeholder="t('tree.namePrompt')"
      :aria-label="t('tree.namePrompt')"
      @keydown.enter="onEnterKey"
    />

    <template #footer>
      <el-button size="default" @click="visible = false">{{ t('common.cancel') }}</el-button>
      <el-button size="default" :disabled="busy" @click="submit('dir')">
        {{ t('tree.newDir') }}
      </el-button>
      <el-button size="default" type="primary" :disabled="busy" @click="submit('file')">
        {{ t('tree.newFile') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElInput } from 'element-plus'
import { useLayoutStore } from '@/stores/layout'
import { onEnter } from '@/utils/keyboard'

/**
 * 「新建」弹窗：**新建目录和新建文件共用一个名字输入框**，动作按钮本身就是「建什么」的
 * 开关——输完名字直接点「新建目录」或「新建文件」，不用先在一处选类型再点确定。
 * 参考 webssh 的 NewEntryDialog；左栏文件树的「新建」是行内输入框，不走这个弹窗。
 *
 * 提交不自我关闭：名字重了 / 目录读不到时由调用方报错、弹窗留着，用户改个名字再点一下
 * 就行，不用重新打开再输一遍。回车 = 新建文件（和 webssh 一致：默认动作是建文件）。
 */
const visible = defineModel<boolean>({ required: true })

defineProps<{
  /** 调用方正在请求，连点两下会建出两个同名文件——按钮先禁掉。 */
  busy?: boolean
}>()

const emit = defineEmits<{ submit: [kind: 'file' | 'dir', name: string] }>()

const { t } = useI18n()
const layout = useLayoutStore()

const name = ref('')
const inputEl = ref<InstanceType<typeof ElInput> | null>(null)

// 每次打开都从空名字开始（与重命名不同，那里要回填原名）
watch(visible, (open) => {
  if (open) name.value = ''
})

/** 等弹窗把输入框渲染出来再聚焦，否则 ref 还是 null。 */
function focusInput() {
  void nextTick(() => inputEl.value?.focus())
}

function submit(kind: 'file' | 'dir') {
  emit('submit', kind, name.value)
}

/** 回车 = 新建文件（组词中的回车不算，见 utils/keyboard.ts）。 */
function onEnterKey(event: Event) {
  onEnter(event, () => submit('file'))
}
</script>
