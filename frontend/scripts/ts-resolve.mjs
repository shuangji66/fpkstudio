// 让 node 直接跑 .ts 源码：tsconfig 用的是 bundler 解析（相对导入不写扩展名、目录
// 走 index.ts），node 的 ESM 解析器要完整路径；'@/' 是 vite / tsconfig 里的别名，
// 指向 src/。
const srcRoot = new URL('../src/', import.meta.url)

export async function resolve(specifier, context, next) {
  const spec = specifier.startsWith('@/')
    ? new URL(specifier.slice(2), srcRoot).href
    : specifier
  try {
    return await next(spec, context)
  } catch (e) {
    if (!spec.endsWith('.ts') && (spec.startsWith('.') || spec.startsWith('file:'))) {
      for (const suffix of ['.ts', '/index.ts']) {
        try {
          return await next(spec + suffix, context)
        } catch {
          /* 试下一个后缀 */
        }
      }
    }
    throw e
  }
}
