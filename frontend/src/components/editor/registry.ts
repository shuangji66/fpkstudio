import type { Component } from 'vue'
import ManifestEditor from './editors/ManifestEditor.vue'
import PrivilegeEditor from './editors/PrivilegeEditor.vue'
import ResourceEditor from './editors/ResourceEditor.vue'
import UiConfigEditor from './editors/UiConfigEditor.vue'
import WizardEditor from './editors/WizardEditor.vue'
import ImagePreview from './editors/ImagePreview.vue'
import SourceEditor from './editors/SourceEditor.vue'

export interface EditorDef {
  name: string
  /** 匹配相对于项目根的路径。 */
  match: RegExp
  component: Component
  /** 是否同时提供源码视图（表单编辑器为真）。 */
  hasForm: boolean
  /**
   * 非文本文件也能渲染（图片预览）。
   *
   * 后端对二进制只回 `text:false`，普通文件这时会落到「这是二进制文件」提示；
   * 标了这个的编辑器照常渲染，自己按路径去取原始内容。
   */
  previews?: boolean
}

/**
 * 编辑器注册表。
 *
 * 与 fnpackup 的做法一致：按路径挑编辑器；区别是匹配规则集中在这里，
 * 组件本身不再持有 match 与弹窗尺寸这类耦合 DOM 的信息。
 * 顺序有意义——先匹配到的先赢，兜底放最后。
 */
export const editors: EditorDef[] = [
  { name: 'manifest', match: /^manifest$/, component: ManifestEditor, hasForm: true },
  { name: 'privilege', match: /^config\/privilege$/, component: PrivilegeEditor, hasForm: true },
  { name: 'resource', match: /^config\/resource$/, component: ResourceEditor, hasForm: true },
  { name: 'ui-config', match: /^app\/[^/]+\/config$/, component: UiConfigEditor, hasForm: true },
  { name: 'wizard', match: /^wizard\//, component: WizardEditor, hasForm: true },
  // 图片一律先预览。svg 是文本，同时给源码视图，方便直接改。
  {
    name: 'image',
    match: /\.(png|jpe?g|gif|webp|bmp|avif|ico)$/i,
    component: ImagePreview,
    hasForm: false,
    previews: true,
  },
  { name: 'svg', match: /\.svg$/i, component: ImagePreview, hasForm: true, previews: true },
  { name: 'source', match: /.*/, component: SourceEditor, hasForm: false },
]

export function resolveEditor(path: string): EditorDef {
  const normalized = path.replace(/^\.?\//, '')
  return editors.find((def) => def.match.test(normalized)) ?? editors[editors.length - 1]!
}
