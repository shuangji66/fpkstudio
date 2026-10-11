import { ref, watch, type Ref } from 'vue'

export interface EditorContent {
  path: string
  content: string
}

/** 每个编辑器组件通过 defineExpose 暴露的接口。 */
export interface EditorApi {
  isDirty(): boolean
  getContent(): EditorContent | null
  reset(): void
}

export interface EditorProps {
  /** 相对项目根的路径，例如 `config/privilege`。 */
  path: string
  /** 用于初始解析 / 加载的文本。表单与源码视图互相切换时，这里放的是切换前的实时内容。 */
  origin: string
  /** 磁盘上的内容，作为「未改动」基线。缺省等于 origin。 */
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
  const baseline = (): string => props.original ?? props.origin

  watch(serialize, (value) => {
    emit('dirty', value !== baseline())
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
    isDirty: () => serialize() !== baseline(),
    getContent: () => ({ path: props.path, content: serialize() }),
    reset: () => {
      model.value = load(props.origin)
    },
  }

  return { model, serialize, api }
}

/** 安全解析 JSON，失败时返回兜底值并把错误交给调用方展示。 */
export function parseJSON<T>(text: string, fallback: T): { value: T; error: string } {
  const trimmed = text.trim()
  if (!trimmed) return { value: fallback, error: '' }
  try {
    return { value: JSON.parse(trimmed) as T, error: '' }
  } catch (e) {
    return { value: fallback, error: e instanceof Error ? e.message : String(e) }
  }
}

/** 按固定键顺序序列化对象，避免每次保存都产生无意义的键序变化。 */
export function stringifyOrdered(value: unknown): string {
  return `${JSON.stringify(sortKeys(value), null, 2)}\n`
}

function sortKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortKeys)
  if (value && typeof value === 'object') {
    const source = value as Record<string, unknown>
    const out: Record<string, unknown> = {}
    for (const key of Object.keys(source).sort()) out[key] = sortKeys(source[key])
    return out
  }
  return value
}
