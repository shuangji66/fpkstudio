/**
 * 首页项目的排版与文案：分页视图「一页放几个」的算术，以及按项目数量给的称号。
 *
 * 两件事放在一起是因为它们只服务于首页项目区，而且都是「算错了肉眼也说不清」的
 * 那类小逻辑。
 *
 * 单独成模块是为了能进 `check:paging` 单独验——这里算错的表现是「一页塞不下，底部
 * 那排卡片被切一半」或者「明明放得下却少放一行」，两种都是肉眼看着像"差不多"、
 * 但结构上确实错了的东西。真实尺寸由 HomeTab 从 DOM 上量（这里不碰 DOM）。
 */
export interface GridFit {
  /** 列数（至少 1）。 */
  cols: number
  /** 行数（至少 1）。 */
  rows: number
  /** 一页放几个 = cols × rows。 */
  perPage: number
  /** 每一列的宽度（按可用宽度等分，扣掉列间距）。 */
  colWidth: number
}

export interface FitOptions {
  /** 卡片的最小列宽（和网格里 minmax 的下限保持一致）。 */
  minWidth?: number
  /** 卡片的最小行高：比它矮就不要再塞一行。 */
  minHeight?: number
  /** 卡片之间的间距（列间距 = 行间距）。 */
  gap?: number
}

/** 首页卡片的默认尺寸约束：17rem 宽、6rem 高、0.75rem 间距（和视图里的类名一致）。 */
export const CARD_MIN_WIDTH = 272
export const CARD_MIN_HEIGHT = 96
export const CARD_GAP = 12

/**
 * 「宽度够放几列、高度够放几行」。
 *
 * 用的是「N 个格子 + (N-1) 个间距」的标准算法：`(可用 + 间距) / (格子 + 间距)` 取整。
 * 可用尺寸为 0 或负数（容器还没量出来、或者被折叠了）时给 1×1 的兜底：宁可先按一页一个
 * 渲染，也不能算出 0 个——那样页面会是空的。
 */
export function fitGrid(availWidth: number, availHeight: number, options: FitOptions = {}): GridFit {
  const minWidth = options.minWidth ?? CARD_MIN_WIDTH
  const minHeight = options.minHeight ?? CARD_MIN_HEIGHT
  const gap = options.gap ?? CARD_GAP

  const cols = Math.max(1, Math.floor((availWidth + gap) / (minWidth + gap)))
  const rows = Math.max(1, Math.floor((availHeight + gap) / (minHeight + gap)))

  return {
    cols,
    rows,
    perPage: cols * rows,
    colWidth: (availWidth - gap * (cols - 1)) / cols,
  }
}

/**
 * 一页的数据被切掉之后，当前页要落在哪一页。
 *
 * 窗口变小（每页能放的卡片变少）或者搜索关键字变了的时候，原来的页码可能已经越界，
 * 这时**退回最后一页**而不是第一页：用户多半是在看列表末尾时缩小了窗口，弹回第一页
 * 等于把他刚看的位置弄丢了。总数变 0 时回到第 1 页。
 */
export function clampPage(page: number, total: number, perPage: number): number {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, perPage)))
  if (!Number.isFinite(page) || page < 1) return 1
  return Math.min(Math.floor(page), pages)
}

/**
 * 项目数量的称号。
 *
 * 区间是**从下往上**判的（先看 ≤1，再看 ≤5……），所以那些叠在边界上的数量归到
 * **下面那一档**：1 个是「无名之辈」，5 个是「小试牛刀」，10 个是「渐入佳境」，
 * 50 个还是「名扬天下」，51 个才是「打包之神」。
 * 负数也走第一档（列表为空时这一行压根不渲染，这里只是不给 undefined）。
 */
const RANK_TIERS: { max: number; key: string }[] = [
  { max: 1, key: 'home.ranks.tier0' },
  { max: 5, key: 'home.ranks.tier1' },
  { max: 10, key: 'home.ranks.tier2' },
  { max: 20, key: 'home.ranks.tier3' },
  { max: 50, key: 'home.ranks.tier4' },
]

export function rankKeyFor(count: number): string {
  for (const tier of RANK_TIERS) {
    if (count <= tier.max) return tier.key
  }
  return 'home.ranks.tier5'
}
