/**
 * 多选下拉（`el-select` multiple）的标签排版：一行里放得下几个标签。
 *
 * Element Plus 的 `collapse-tags` 是**按个数**折叠的（`max-collapse-tags`），压根不看
 * 容器有多宽：默认值 1 的意思就是「选了 4 个也只显示第 1 个，剩下的收进 +3」，
 * 哪怕输入框有 600px 宽、4 个短标签加起来才 300px。所以由调用方把「能放下几个」
 * 量出来（`TagSelect.vue` 里量），这里只做算术。
 *
 * 算错的代价：多算一个 → 标签折到第二行；少算一个 → 明明放得下却白折叠一个。
 * 都不至于写坏用户的文件，但看着就是坏掉了，所以单独成函数、进 `check:tags`。
 */

/**
 * @param widths 每个标签的宽度（像素，按显示顺序）
 * @param available 这一行留给标签的总宽度（已扣掉输入框和间距）
 * @param gap 标签之间的间距
 * @param collapseWidth 「+N」那个标签自身的宽度；放得下时它是 0，不影响结果
 * @returns 最多显示几个标签（至少 1；返回 `widths.length` 表示全都放得下、不用折叠）
 */
export function fitTagCount(
  widths: number[],
  available: number,
  gap: number,
  collapseWidth: number,
): number {
  const total = widths.length
  if (total === 0) return 0

  const used = (count: number) =>
    widths.slice(0, count).reduce((sum, width) => sum + width, 0) + gap * Math.max(0, count - 1)

  // 全放得下就别折叠——「+N」不出现，也就不占位置
  if (used(total) <= available) return total

  // 放不下才给「+N」留位置：它自己也要占一个 gap，宽度随 N 的位数变
  for (let count = total - 1; count >= 1; count -= 1) {
    if (used(count) + gap + collapseWidth <= available) return count
  }
  // 连一个标签 + 「+N」都放不下：给 1，让 el-select 自己去省略号（它会给标签挂 max-width）
  return 1
}
