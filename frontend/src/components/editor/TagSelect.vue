<template>
  <el-select
    ref="selectRef"
    :model-value="modelValue"
    :size="size ?? 'small'"
    multiple
    filterable
    allow-create
    default-first-option
    collapse-tags
    collapse-tags-tooltip
    :max-collapse-tags="maxCollapseTags"
    :reserve-keyword="false"
    :placeholder="placeholder"
    :popper-class="popperClass"
    @update:model-value="onUpdate"
  >
    <el-option v-for="option in normalized" :key="option.value" :label="option.label" :value="option.value" />
  </el-select>
</template>

<script setup lang="ts">
/**
 * 多选标签下拉：**宽度够就把选中的标签全显示出来，放不下才折叠成 +N**。
 *
 * `el-select` 的 `collapse-tags` 是按**个数**折叠的（`max-collapse-tags`，默认 1），
 * 根本不看容器有多宽——一个 600px 宽、四个短标签的输入框也会显示成「第 1 个 +3」。
 * 所以这里每轮排版都量一次标签的真实宽度，用 `fitTagCount` 算出放得下几个再交给
 * `el-select`。宽度变化靠 ResizeObserver 跟（桌面 / 移动布局会换宽度，编辑器的标签页
 * 也可能一直是 v-show 藏着的）。
 *
 * 两个刻意的选择：
 *  - 初值取「一个都不折叠」，量完再收紧。反过来的话（初值 1 再放开）首帧会闪一下 +N。
 *  - 量宽度前先把标签全部渲染出来（折叠掉的标签不在 DOM 里，量不到），所以一轮排版
 *    会有两次渲染。两次都在同一个微任务队列里跑完，浏览器还没绘制。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElSelect } from 'element-plus'
import { fitTagCount } from '@/utils/tags'

interface TagSelectOption {
  label: string
  value: string
}

interface Props {
  modelValue: string[]
  /** 候选：给字符串就是「值即标签」，给对象可以另配显示的 label。 */
  options: (string | TagSelectOption)[]
  placeholder?: string
  popperClass?: string
  /** el-select 的尺寸；默认 small（全局配的也是 small）。 */
  size?: 'small' | 'default' | 'large'
}

const props = defineProps<Props>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()

/** 「一个都不折叠」的哨兵值：`selected.length > maxCollapseTags` 恒不成立，+N 不出现。 */
const NO_COLLAPSE = Number.MAX_SAFE_INTEGER

const selectRef = ref<InstanceType<typeof ElSelect>>()
const maxCollapseTags = ref(NO_COLLAPSE)

const normalized = computed<TagSelectOption[]>(() =>
  props.options.map((option) => (typeof option === 'string' ? { label: option, value: option } : option)),
)

function onUpdate(value: unknown) {
  emit('update:modelValue', Array.isArray(value) ? value.map(String) : [])
}

const root = () => selectRef.value?.$el as HTMLElement | undefined

/**
 * 一轮排版：渲染 → 量宽度 → 算能放几个 → 复核。
 *
 * `run` 是防串号的令牌：`await` 之后可能有更新的一轮在跑，那就让位，别拿旧数据写回。
 */
let run = 0

async function relayout() {
  const token = (run += 1)
  const stale = () => token !== run

  const el = root()
  const selection = el?.querySelector<HTMLElement>('.el-select__selection')
  const count = props.modelValue.length
  if (!selection || count === 0) {
    maxCollapseTags.value = NO_COLLAPSE
    return
  }
  // 宽度为 0 = 这个编辑器在 v-show 藏着的标签页里，量不出东西。先不折叠（标签会折行），
  // 等 ResizeObserver 报到真实宽度再收紧。
  if (selection.clientWidth === 0) {
    maxCollapseTags.value = NO_COLLAPSE
    return
  }

  // 折叠掉的标签不在 DOM 里，量不到宽度，所以先全部渲染出来
  if (maxCollapseTags.value !== NO_COLLAPSE) {
    maxCollapseTags.value = NO_COLLAPSE
    await nextTick()
    if (stale()) return
  }
  const items = tagItems(selection)
  if (items.length !== count) return
  const widths = items.map((item) => item.getBoundingClientRect().width)

  // 这一行留给标签的宽度 = 选择区宽度 - 行末输入框的最小宽度 - 它前面那个 gap。
  // 输入框在「多选 + 没聚焦」时是绝对定位的（`is-hidden`，不占位置），但一聚焦就回到行里，
  // 所以这里按它在流里的最小宽度扣——否则点一下输入框，标签就折到第二行去了。
  // 11px 是 `el-select` 自己写在 CSS 里的最小宽度，它的 tagStyle 里也是按 11 扣的。
  const gap = Number.parseFloat(getComputedStyle(selection).gap) || 0
  const input = selection.querySelector<HTMLElement>('.el-select__input')
  const inputMin = input ? Number.parseFloat(getComputedStyle(input).minWidth) || 11 : 11
  const available = selection.clientWidth - inputMin - gap

  let fitted = fitTagCount(widths, available, gap, 0)
  for (let pass = 0; pass < 4; pass += 1) {
    maxCollapseTags.value = fitted >= count ? NO_COLLAPSE : fitted
    if (fitted >= count) break // 全放得下，没有 +N 可量，到此为止
    await nextTick()
    if (stale()) return
    // 算得对不对，看真的渲染结果：折到第二行就再让一个出去
    if (wrapped(selection)) {
      fitted = Math.max(1, fitted - 1)
      continue
    }
    // 「+N」自己也要占位置，而它的宽度要看 N 有几位——量到真的再复核一次
    const collapse = selection.querySelector<HTMLElement>('.el-select__collapse-item')
    const next = fitTagCount(widths, available, gap, collapse?.getBoundingClientRect().width ?? 0)
    if (next >= fitted) break
    fitted = next
  }
}

/**
 * 选择区里的标签元素。
 *
 * `.el-select__selected-item` 这个名字下有三种东西：标签、折叠成的「+N」、以及行末那个
 * 输入框的壳（多选收起时它挂着 `is-hidden` 绝对定位）。后两种都不是标签，要排掉。
 */
function tagItems(selection: HTMLElement): HTMLElement[] {
  return [
    ...selection.querySelectorAll<HTMLElement>(
      '.el-select__selected-item:not(.el-select__collapse-item):not(.el-select__input-wrapper)',
    ),
  ]
}

/** 标签是不是排到了第二行（顶边不止一个）。只看标签：输入框那个壳是绝对定位的，不参与。 */
function wrapped(selection: HTMLElement): boolean {
  const tops = new Set(tagItems(selection).map((el) => Math.round(el.getBoundingClientRect().top)))
  return tops.size > 1
}

let observer: ResizeObserver | undefined
/** 上一次量到的宽度：只有宽度变了才重排，否则自己折叠引起的高度变化会把 RO 打成死循环。 */
let lastWidth = -1

onMounted(() => {
  void relayout()
  const el = root()
  if (!el || typeof ResizeObserver === 'undefined') return
  observer = new ResizeObserver((entries) => {
    const width = Math.round(entries[0]?.contentRect.width ?? -1)
    if (width === lastWidth) return
    lastWidth = width
    void relayout()
  })
  observer.observe(el)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  run += 1 // 让在跑的那一轮停手，别再动已经卸载的 DOM
})

watch(() => [props.modelValue, props.options], () => void relayout(), { deep: true })
</script>
