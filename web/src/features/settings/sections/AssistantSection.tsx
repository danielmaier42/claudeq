import {Row, RowGroup} from '@/components/layout/settings-row'
import {Section} from '@/components/layout/view-layout'
import {ModelSelect} from '@/components/domain/model-select'
import {Select, type SelectOption} from '@/components/ui/select'
import {Switch} from '@/components/ui/switch'
import {defaultProvider, providerName, useBeta, useProviderList} from '@/lib/providers'
import type {Settings} from '@/lib/types'
import {commitSettings} from '../actions'
import {SaveMode} from '../components/parts'

// asideProviderOptions lists the providers that can do a job ClaudeQ runs on
// its own behalf (prompt review, the feedback assistant). Only a harness that
// can answer such a question is offered: one that cannot would leave the
// feature silently unavailable. A stored provider that is gone keeps its place,
// marked, so nothing quietly moves the job somewhere else.
function useAsideProviderOptions(current: string): SelectOption[] {
  const providers = useProviderList()
  const beta = useBeta()
  const offered = providers.filter((p) => p.enabled && p.asides && (!p.beta || beta))
  const dflt = defaultProvider(providers)
  const opts: SelectOption[] = [
    {value: '', label: dflt ? `Default (${providerName(dflt)})` : 'Default provider'},
    ...offered.map((p) => ({value: p.id, label: providerName(p) + (p.beta ? ' (beta)' : '')})),
  ]
  if (current && !offered.some((p) => p.id === current))
    opts.push({value: current, label: `${current} (unavailable)`})
  return opts
}

export function AssistantSection({value}: {value: Settings}) {
  const reviewOn = !value.prompt_review_disabled
  const reviewProviders = useAsideProviderOptions(value.prompt_review_provider)
  const feedbackProviders = useAsideProviderOptions(value.feedback_provider)

  return (
    <>
      <Section title="Prompt review" actions={<SaveMode mode="instant" />}>
        <RowGroup>
          <Row
            htmlFor="s-review"
            label="Check prompts with Claude"
            description="Before a task is added, imported or edited, Claude reads its prompt against this Mac: paths that aren't here, a file to be written into a folder that doesn't exist, guidelines it points at. It offers a rewrite under the prompt box, and reviews the custom system prompt the same way."
            control={
              <Switch
                id="s-review"
                checked={reviewOn}
                onCheckedChange={(on) => void commitSettings({prompt_review_disabled: !on})}
              />
            }
          />
          <Row
            htmlFor="s-review-provider"
            label="Review provider"
            description="Reviewing costs allowance too, so it can run on an account other than the one your tasks use"
            control={
              <Select
                id="s-review-provider"
                className="w-56"
                disabled={!reviewOn}
                value={value.prompt_review_provider}
                options={reviewProviders}
                // A model name means nothing to another harness, so it goes too.
                onValueChange={(v) =>
                  void commitSettings({prompt_review_provider: v, prompt_review_model: ''})
                }
              />
            }
          />
          <Row
            htmlFor="s-review-model"
            label="Review model"
            description="Runs on every prompt change, so a fast model earns its keep"
            control={
              <ModelSelect
                id="s-review-model"
                className="w-56"
                disabled={!reviewOn}
                provider={value.prompt_review_provider}
                value={value.prompt_review_model}
                emptyLabel="The provider's default model"
                onValueChange={(v) => void commitSettings({prompt_review_model: v})}
              />
            }
          />
        </RowGroup>
      </Section>

      <Section title="Feedback assistant" actions={<SaveMode mode="instant" />}>
        <RowGroup>
          <Row
            htmlFor="s-feedback-provider"
            label="Feedback provider"
            description="Drafts the GitHub issue on the Feedback page. Nothing is sent anywhere: the draft opens as a prefilled issue in your browser, and you press Create there."
            control={
              <Select
                id="s-feedback-provider"
                className="w-56"
                value={value.feedback_provider}
                options={feedbackProviders}
                onValueChange={(v) => void commitSettings({feedback_provider: v, feedback_model: ''})}
              />
            }
          />
          <Row
            htmlFor="s-feedback-model"
            label="Feedback model"
            description="Two short messages into an issue: a small model is enough"
            control={
              <ModelSelect
                id="s-feedback-model"
                className="w-56"
                provider={value.feedback_provider}
                value={value.feedback_model}
                emptyLabel="ClaudeQ's choice (a small, fast model)"
                onValueChange={(v) => void commitSettings({feedback_model: v})}
              />
            }
          />
        </RowGroup>
      </Section>
    </>
  )
}
