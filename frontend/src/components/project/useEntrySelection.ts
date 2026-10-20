import { ref } from 'vue'

/**
 * 文件管理器（左栏文件树 / 「文件」标签页）的多选。
 *
 * 两个视图的单击语义本来就不同（树里点目录是展开，列表里点目录是进去），所以
 * 这里只收「哪些路径被选中」和修饰键的判断，真正的动作由调用方作为 `activate`
 * 传进来：
 *
 *  - 普通点击：只选这一项，然后执行 activate（打开 / 展开 / 进目录）。
 *  - Ctrl / Cmd 点击：切换这一项，**不执行** activate——多选时点一下文件就把它
 *    开成标签，选出来的东西全被标签页盖住了。
 *  - Shift 点击：从上次点击的那一项（锚点）选到这一项（按当前可见顺序）。
 *
 * 右键不改多选：点在已选中的某一项上时保留整批选择（菜单据此对整批做复制 / 剪切 /
 * 删除），点在别的项上才收敛成只选它。
 */
export function useEntrySelection(visiblePaths: () => string[]) {
  const selected = ref<string[]>([])
  /** 上次普通 / 修饰键点击的落点，Shift 范围从这里算起。 */
  const anchor = ref('')

  const isSelected = (path: string) => selected.value.includes(path)

  function clear() {
    selected.value = []
    anchor.value = ''
  }

  function setOnly(path: string) {
    selected.value = [path]
    anchor.value = path
  }

  function toggle(path: string) {
    selected.value = isSelected(path)
      ? selected.value.filter((item) => item !== path)
      : [...selected.value, path]
    anchor.value = path
  }

  /** 普通 / Ctrl / Shift 点击的共用入口。带修饰键时调用方的 activate 不会执行。 */
  function click(path: string, event: MouseEvent, activate: () => void) {
    if (event.shiftKey && anchor.value) {
      selected.value = rangeBetween(visiblePaths(), anchor.value, path)
      return
    }
    if (event.ctrlKey || event.metaKey) {
      toggle(path)
      return
    }
    setOnly(path)
    activate()
  }

  /** 右键：落在已选中的一项上就保留整批选择，否则只选这一项。 */
  function context(path: string) {
    if (!isSelected(path)) setOnly(path)
    else anchor.value = path
  }

  /**
   * 丢掉已经不在列表里的选择。
   *
   * 目录刷新、收起目录、删除之后都要过一遍：否则选择里会一直留着已经不存在的路径，
   * 批量删除还会对一条不存在的路径发请求。
   */
  function prune() {
    const alive = new Set(visiblePaths())
    selected.value = selected.value.filter((path) => alive.has(path))
    if (anchor.value && !alive.has(anchor.value)) anchor.value = ''
  }

  return { selected, isSelected, clear, setOnly, click, context, prune }
}

/**
 * 按可见顺序取锚点到目标之间的一段（含两端）。
 *
 * 锚点已经不在列表里（目录被收起 / 删掉）时退化成「只选目标」：硬算一个 from 到 to
 * 的字符串区间只会选出一批不在当前目录里的路径。
 */
function rangeBetween(all: string[], from: string, to: string): string[] {
  const a = all.indexOf(from)
  const b = all.indexOf(to)
  if (a < 0 || b < 0) return [to]
  return a <= b ? all.slice(a, b + 1) : all.slice(b, a + 1)
}

/**
 * 批量删除前把「父目录和它里面的子项」收敛成父目录一项。
 *
 * 文件树里同时选中 `building/` 和 `building/platform` 是可能的（Ctrl 点两次），
 * 逐条删的话第二条会对着一个已经不存在（或者已经换内容）的路径操作。
 * 保持原有顺序，只去掉被别的选中项包含的那些。
 */
export function dropNestedPaths(paths: string[]): string[] {
  return paths.filter(
    (path) => !paths.some((other) => other !== path && path.startsWith(`${other}/`)),
  )
}
