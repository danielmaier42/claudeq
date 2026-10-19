import {
  File,
  FileArchive,
  FileBraces,
  FileCode,
  FileImage,
  FileSpreadsheet,
  FileText,
  type LucideIcon,
} from 'lucide-react'
import {cn} from '@/lib/utils'
import {artifactKind, extLabel} from './logic'

interface FileLook {
  icon: LucideIcon
  // Tile background and glyph colour, from the theme's status tones so both
  // appearances and every accent stay legible.
  tone: string
}

const ARCHIVES = new Set(['ZIP', 'TAR', 'GZ', 'TGZ', 'BZ2', 'XZ', '7Z', 'RAR'])
const SHEETS = new Set(['CSV', 'TSV', 'XLS', 'XLSX', 'NUMBERS'])
const DATA = new Set(['JSON', 'JSONL', 'NDJSON', 'XML', 'YAML', 'YML', 'TOML'])
const CODE = new Set(['JS', 'TS', 'PY', 'GO', 'SH', 'SQL', 'CSS'])

// fileLook gives each type of file its own glyph and colour, so a grid of cards
// sorts itself by eye: red PDFs, blue web pages, purple images.
export function fileLook(fileName: string, contentType: string): FileLook {
  const ext = extLabel(fileName, contentType)
  const kind = artifactKind(contentType)
  if (kind === 'pdf') return {icon: FileText, tone: 'bg-danger-soft text-danger'}
  if (kind === 'html') return {icon: FileCode, tone: 'bg-info-soft text-info'}
  if (kind === 'image') return {icon: FileImage, tone: 'bg-ai-soft text-ai'}
  if (SHEETS.has(ext)) return {icon: FileSpreadsheet, tone: 'bg-success-soft text-success'}
  if (DATA.has(ext)) return {icon: FileBraces, tone: 'bg-warning-soft text-warning'}
  if (CODE.has(ext)) return {icon: FileCode, tone: 'bg-accent-soft text-accent-text'}
  if (ARCHIVES.has(ext)) return {icon: FileArchive, tone: 'bg-surface-3 text-muted-foreground'}
  if (kind === 'text') return {icon: FileText, tone: 'bg-surface-3 text-muted-foreground'}
  return {icon: File, tone: 'bg-surface-3 text-muted-foreground'}
}

export function FileIcon({
  fileName,
  contentType,
  size = 'md',
  className,
}: {
  fileName: string
  contentType: string
  size?: 'sm' | 'md'
  className?: string
}) {
  const {icon: Icon, tone} = fileLook(fileName, contentType)
  return (
    <div
      aria-hidden
      className={cn(
        'flex shrink-0 items-center justify-center rounded-lg',
        size === 'sm' ? 'size-7 [&_svg]:size-3.5' : 'size-9 [&_svg]:size-[18px]',
        tone,
        className,
      )}
    >
      <Icon />
    </div>
  )
}
