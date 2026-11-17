export type Attrs = Record<string, string | number | boolean | null | undefined>
export type Child = Node | string | number | null | undefined | false

/**
 * Tiny element factory. Every node this component builds goes through here, so
 * the module never assigns HTML strings: settings text, template previews, and
 * route payloads all land as text nodes.
 */
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Attrs = {},
  ...children: Child[]
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  for (const [name, value] of Object.entries(attrs)) {
    if (value === null || value === undefined || value === false) continue
    if (value === true) {
      node.setAttribute(name, '')
      continue
    }
    node.setAttribute(name, String(value))
  }
  for (const child of children) {
    if (child === null || child === undefined || child === false) continue
    node.append(child instanceof Node ? child : document.createTextNode(String(child)))
  }
  return node
}
