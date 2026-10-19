import {Folder, FolderOpen, X} from 'lucide-react'
import {useState} from 'react'
import {RowGroup} from '@/components/layout/settings-row'
import {Section} from '@/components/layout/view-layout'
import {Button} from '@/components/ui/button'
import {Textarea} from '@/components/ui/input'
import {toast} from '@/components/ui/toaster'
import {Tooltip} from '@/components/ui/tooltip'
import {PromptReview} from '@/features/task-editor/review/PromptReview'
import {errorMessage, post} from '@/lib/api'
import type {PathResult, Settings} from '@/lib/types'
import {commitSettings} from '../actions'
import {SaveMode} from '../components/parts'
import {editDraft} from '../store'

export function GeneralSection({value, stored}: {value: Settings; stored: Settings}) {
  return (
    <>
      <Section
        title="Custom system prompt"
        description="Appended to every run after DispatchQ's built-in instructions (built-in first, yours last)"
        actions={<SaveMode mode="draft" dirty={value.system_prompt !== stored.system_prompt} />}
      >
        <RowGroup>
          <div className="px-4 py-3">
            <Textarea
              id="s-sysprompt"
              aria-label="Custom system prompt"
              rows={7}
              value={value.system_prompt}
              onChange={(e) => editDraft({system_prompt: e.target.value})}
              placeholder="e.g. Follow the repo's existing conventions. Always run the tests before finishing."
            />
            <p className="mt-2 text-[12px] text-muted-foreground">
              Standing guidance you want on every task: coding conventions, tone, tools to prefer. Leave empty
              for none.
            </p>
            {/* The system prompt has no working directory: it is appended to
                every task, wherever that task runs. */}
            <PromptReview
              kind="system"
              prompt={value.system_prompt}
              workingDir=""
              onApply={(text) => editDraft({system_prompt: text})}
              className="mt-3"
            />
          </div>
        </RowGroup>
      </Section>

      <Section
        title="Default working directory"
        description="A new task's working directory starts here. Still editable per task."
        actions={<SaveMode mode="instant" />}
      >
        <RowGroup>
          <DefaultDirRow dir={stored.default_working_dir} />
        </RowGroup>
      </Section>
    </>
  )
}

function DefaultDirRow({dir}: {dir: string}) {
  const [choosing, setChoosing] = useState(false)

  const choose = async () => {
    setChoosing(true)
    try {
      const q = dir ? '?path=' + encodeURIComponent(dir) : ''
      // 204 (undefined) means the dialog was cancelled.
      const r = await post<PathResult | undefined>('/api/fs/choose' + q)
      if (r?.path && r.path !== dir) {
        if (await commitSettings({default_working_dir: r.path})) toast.success('Default directory set')
      }
    } catch (e) {
      toast.error('Folder dialog unavailable', {description: errorMessage(e)})
    } finally {
      setChoosing(false)
    }
  }

  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3/70 text-muted-foreground">
        <Folder className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        {dir ? (
          <Tooltip content={dir}>
            <div className="truncate font-mono text-[12.5px]" dir="rtl">
              <bdi>{dir}</bdi>
            </div>
          </Tooltip>
        ) : (
          <div className="text-[13px] text-muted-foreground">
            No default: new tasks start with an empty field
          </div>
        )}
      </div>
      {dir && (
        <Tooltip content="Clear the default">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Clear the default directory"
            onClick={() => void commitSettings({default_working_dir: ''})}
          >
            <X />
          </Button>
        </Tooltip>
      )}
      <Button onClick={() => void choose()} loading={choosing}>
        {!choosing && <FolderOpen />}
        {dir ? 'Change…' : 'Choose folder…'}
      </Button>
    </div>
  )
}
