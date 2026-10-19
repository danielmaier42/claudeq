import {Select, type SelectOption} from '@/components/ui/select'
import {useModels} from '@/lib/queries'

// ModelSelect offers what a provider suggests (or the global catalog with no
// provider). A stored model the list does not contain is kept and marked
// custom: a catalog is a suggestion, and a setting must never lose the model it
// was given because discovery came back short.
export function ModelSelect({
  provider = '',
  value,
  onValueChange,
  emptyLabel,
  disabled,
  id,
  className,
}: {
  provider?: string
  value: string
  onValueChange: (v: string) => void
  emptyLabel: string
  disabled?: boolean
  id?: string
  className?: string
}) {
  const models = useModels(provider).data ?? []
  const options: SelectOption[] = [
    {value: '', label: emptyLabel},
    ...models.map((m) => ({value: m.id, label: m.label})),
  ]
  if (value && !models.some((m) => m.id === value)) options.push({value, label: `${value} (custom)`})
  return (
    <Select
      id={id}
      className={className}
      value={value}
      onValueChange={onValueChange}
      options={options}
      disabled={disabled}
    />
  )
}
