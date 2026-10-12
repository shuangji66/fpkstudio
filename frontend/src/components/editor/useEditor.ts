import { computed, ref, watch, type Ref } from 'vue'

export interface EditorContent {
  path: string
  content: string
}

/**
 * 每个编辑器组件通过 defineExpose 暴露的接口。
 *
 * 组件里必须 `const { model, api } = useEditor(...)` 之后 `defineExpose(api)`：
 * `<script setup>` 默认是封闭的，不 expose 的话 EditorHost 拿不到 getContent，
 * 「保存」会静默失效（表单编辑器全都踩过这个坑）。EditorHost 现在会在取不到
 * 内容时报错，但还是别忘了 expose。
 */
export interface EditorApi {
  isDirty(): boolean
  getContent(): EditorContent | null
  /** 可选：外部整体写入内容（「插入模板」这类按钮用）。 */
  setContent?(text: string): void
  /**
   * 可选：返回一句「为什么现在不能保存」，空串表示可以保存。
   *
   * EditorHost 在取内容之前先问这个。向导文件解析失败时表单里是空的，这时候
   * `getContent()` 交出去的会是一份空向导——保存等于把用户的文件清空，所以要在
   * 那之前拦住。
   */
  saveBlocked?(): string
}

export interface EditorProps {
  /** 相对项目根的路径，例如 `config/privilege`。 */
  path: string
  /** 用于初始解析 / 加载的文本。表单与源码视图互相切换时，这里放的是切换前的实时内容。 */
  origin: string
  /**
   * 磁盘上的内容。缺省等于 origin。
   *
   * 「未改动」是相对它算的，但比的是**它走一遍解析 + 回写之后的结果**（见 useEditor
   * 里的 baseline），不是逐字节相等。
   */
  original?: string
  /** 所属项目名，供编辑器做上下文相关的校验提示。 */
  project?: string
}

/**
 * 表单类编辑器的公共骨架。
 *
 * 统一处理三件事：把文本解析成模型、把模型序列化回文本、按序列化结果判断是否
 * 有改动。用 getter 形式的 watch 而不是定时轮询——Vue 只在依赖真的变化时才回调，
 * 不会像 fnpackup 那样每 500ms 无条件比较一次。
 */
export function useEditor<M>(
  props: EditorProps,
  emit: (event: 'dirty', value: boolean) => void,
  load: (text: string) => M,
  dump: (model: M) => string,
) {
  const model = ref(load(props.origin)) as Ref<M>

  const serialize = (): string => dump(model.value)

  /**
   * 「未改动」的基线：**磁盘上那份文本走一遍同样的解析 + 回写**，而不是原文本身。
   *
   * 回写本来就会把格式归一（JSON 的缩进与键序、manifest 补默认键），所以拿原文当
   * 基线的话，一份**没编辑过**的脚手架文件（fnpack 生成的 `config/privilege` 是
   * 4 空格缩进、结尾不带换行）在挂载那一刻就算「脏」。表单视图看不出来——这个 watch
   * 不是 immediate，而 EditorHost 挂载时不问 isDirty；但源码视图的判据是「文本 ≠
   * 磁盘原文」，一按「源码」就凭空显示「未保存」。
   *
   * 基线走同一条归一，脏标记才真的等于「模型被改过」：没编辑过是干净的，改了再改
   * 回来也会自己变干净。基线是磁盘内容的纯函数，所以保存 / 还原换了 origin 之后
   * 会跟着重算。
   */
  const baseline = computed(() => dump(load(props.original ?? props.origin)))

  watch(serialize, (value) => {
    emit('dirty', value !== baseline.value)
  })

  // 外部内容变化（例如「还原」后重新读取）时重新解析。
  watch(
    () => props.origin,
    (text) => {
      model.value = load(text)
      emit('dirty', false)
    },
  )

  const api: EditorApi = {
    isDirty: () => serialize() !== baseline.value,
    getContent: () => ({ path: props.path, content: serialize() }),
    // 「从别的向导复制」这类整体替换。解析失败由各自的 load 兜住（返回空模型 + error），
    // 所以这里不需要再判断。
    setContent: (text: string) => {
      model.value = load(text)
    },
  }

  return { model, api }
}

// 这两个是「JSON 配置怎么写回磁盘」的约定，放在 utils 里，表单编辑器和
// utils/wizard.ts 共用一份实现（这里转出去，是为了让编辑器只 import 一个模块）。
export { parseJSON, stringifyOrdered } from '@/utils/json'
