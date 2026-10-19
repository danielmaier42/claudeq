// matchesWords: every word of the query occurs somewhere in the text, so
// "adr 0050" finds "ADR-0050 review" and "seo pdf" the PDF of the SEO job.
export function matchesWords(query: string, ...texts: (string | undefined | null)[]): boolean {
  const words = (query || '').toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return true
  const h = texts.filter(Boolean).join(' ').toLowerCase()
  return words.every((w) => h.includes(w))
}
