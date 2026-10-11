// 让 node 直接跑 .ts 源码：tsconfig 用的是 bundler 解析（相对导入不写扩展名），
// node 的 ESM 解析器需要扩展名，这里在解析失败时补一个 .ts 再试。
export async function resolve(specifier, context, next) {
  try {
    return await next(specifier, context)
  } catch (e) {
    if (specifier.startsWith('.') && !specifier.endsWith('.ts')) {
      return next(`${specifier}.ts`, context)
    }
    throw e
  }
}
