/**
 * 虚拟键盘 / 可视视口适配。
 *
 * 手机上点开输入框、键盘从底下顶上来时，**布局视口并不会自己变矮**：
 *
 *  - iOS Safari 只缩「可视视口」，布局视口和 `window.innerHeight` 都不动；
 *  - Android Chrome 默认也只缩可视视口（要它连布局一起重排，得靠 viewport meta 里
 *    的 `interactive-widget=resizes-content`，而且只有 Chromium 系认）。
 *
 * 而这个应用整块高度是写死的 `100%`，键盘一起来，底部那一截（正在编辑的输入框、
 * 编辑器最后几行、弹窗底部的按钮）就被盖住了。
 *
 * 更麻烦的是 iOS：键盘弹起时它允许**把可视视口拖着平移**（好让你看键盘后面那块）。
 * 只把根容器的高度减掉键盘高度是不够的——可视视口一平移，下面就露出空白，表现就是
 * 「拖到底还能继续拖，页面像变大了」。所以这里把可视视口本身（高度 + 上沿偏移）写成
 * CSS 变量，由 style.css 把根容器**钉在可视视口上**：可视的那一块永远被应用盖满，
 * 没地方可拖。弹窗遮罩同理（它是 position: fixed 的，不跟着根容器走）。
 *
 * 公式对三种情况都成立：
 *
 *  - iOS：`innerHeight` 不变、可视视口变矮 → 变量跟着可视视口走；
 *  - 认 meta 的 Android：`innerHeight` 跟着变矮 → 变量和它一致，不会重复缩；
 *  - 桌面：可视视口就是整个视口，等于没变。
 *
 * 所以它**不需要判断「现在是不是移动布局」**——有没有键盘由浏览器说了算，桌面上这个
 * 监听等于白挂，代价可以忽略。这也是为什么它挂在 main.ts 上而不是某个视图里。
 */
export function initViewportAdaptation(): void {
  const vv = window.visualViewport
  // 老浏览器没有这个 API：那就只能靠 viewport meta（Android）兜着。
  if (!vv) return

  const root = document.documentElement

  const apply = () => {
    // 捏合缩放时可视视口也会变矮，但那不是键盘，不该把页面缩掉。
    const zoomed = vv.scale !== 1
    const height = zoomed ? window.innerHeight : Math.round(vv.height)
    const top = zoomed ? 0 : Math.round(vv.offsetTop)
    root.style.setProperty('--vv-height', `${height}px`)
    root.style.setProperty('--vv-top', `${top}px`)
  }

  vv.addEventListener('resize', apply)
  // 键盘弹出、以及 iOS 平移可视视口时，都只有 scroll 事件。
  vv.addEventListener('scroll', apply)
  window.addEventListener('orientationchange', apply)

  apply()
}
