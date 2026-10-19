// A small, safe reading of the markdown an agent writes: fenced code, headings,
// lists and paragraphs, with inline code, bold and links. It only produces
// plain data the renderer turns into React elements, never HTML.

export type Block =
  | {kind: 'code'; lang: string; text: string}
  | {kind: 'heading'; text: string}
  | {kind: 'list'; ordered: boolean; items: string[]}
  | {kind: 'quote'; text: string}
  | {kind: 'para'; text: string}

export type Inline =
  {kind: 'text' | 'code' | 'bold'; text: string} | {kind: 'link'; text: string; href: string}

const FENCE = /^\s*(```|~~~)\s*([\w+-]*)\s*$/
const BULLET = /^\s*[-*•]\s+/
const NUMBER = /^\s*\d+[.)]\s+/

export function parseBlocks(src: string): Block[] {
  const lines = src.replace(/\r\n/g, '\n').split('\n')
  const out: Block[] = []
  let para: string[] = []
  const flush = () => {
    if (para.length) out.push(...paraBlocks(para))
    para = []
  }
  for (let i = 0; i < lines.length; i++) {
    const fence = FENCE.exec(lines[i])
    if (!fence) {
      if (lines[i].trim()) para.push(lines[i])
      else flush()
      continue
    }
    flush()
    const body: string[] = []
    // An unclosed fence runs to the end: a live log may stop mid-block.
    for (i++; i < lines.length && !lines[i].trim().startsWith(fence[1]); i++) body.push(lines[i])
    out.push({kind: 'code', lang: fence[2], text: body.join('\n')})
  }
  flush()
  return out
}

// One run of non-blank lines: headings stand alone, a list is consecutive list
// lines, a quote consecutive "> " lines, everything else a paragraph.
function paraBlocks(lines: string[]): Block[] {
  const out: Block[] = []
  let text: string[] = []
  const flushText = () => {
    if (text.length) out.push({kind: 'para', text: text.join('\n')})
    text = []
  }
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i]
    const h = /^\s*#{1,6}\s+(.*)$/.exec(l)
    if (h) {
      flushText()
      out.push({kind: 'heading', text: h[1]})
      continue
    }
    const ordered = NUMBER.test(l)
    if (ordered || BULLET.test(l)) {
      flushText()
      const re = ordered ? NUMBER : BULLET
      const items: string[] = []
      for (; i < lines.length; i++) {
        if (re.test(lines[i])) items.push(lines[i].replace(re, ''))
        else if (/^\s{2,}\S/.test(lines[i]) && items.length) items[items.length - 1] += '\n' + lines[i].trim()
        else break
      }
      i--
      out.push({kind: 'list', ordered, items})
      continue
    }
    if (/^\s*>\s?/.test(l)) {
      flushText()
      const q: string[] = []
      for (; i < lines.length && /^\s*>\s?/.test(lines[i]); i++) q.push(lines[i].replace(/^\s*>\s?/, ''))
      i--
      out.push({kind: 'quote', text: q.join('\n')})
      continue
    }
    text.push(l)
  }
  flushText()
  return out
}

const INLINE =
  /`([^`\n]+)`|\*\*([^*\n]+)\*\*|\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])/g

export function parseInline(src: string): Inline[] {
  const out: Inline[] = []
  let last = 0
  for (const m of src.matchAll(INLINE)) {
    const at = m.index ?? 0
    if (at > last) out.push({kind: 'text', text: src.slice(last, at)})
    if (m[1] !== undefined) out.push({kind: 'code', text: m[1]})
    else if (m[2] !== undefined) out.push({kind: 'bold', text: m[2]})
    else if (m[3] !== undefined) out.push({kind: 'link', text: m[3], href: m[4]})
    else out.push({kind: 'link', text: m[5], href: m[5]})
    last = at + m[0].length
  }
  if (last < src.length) out.push({kind: 'text', text: src.slice(last)})
  return out
}
