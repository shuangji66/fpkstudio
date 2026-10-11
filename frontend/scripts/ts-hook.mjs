/**
 * 让 node 能直接跑 src 下的 .ts：tsconfig 用的是 bundler 解析（相对导入不写扩展名），
 * node 的 ESM 解析器要求扩展名，这里在解析失败时补一个 .ts 再试。
 *
 *   node --import ./scripts/ts-hook.mjs scripts/check-xxx.mjs
 */
import { register } from 'node:module'

register('./ts-resolve.mjs', import.meta.url)
