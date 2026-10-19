import {LayoutGrid, List} from 'lucide-react'
import {Segmented} from '@/components/ui/segmented'
import {setArtifactLayout, useArtifactLayout, type ArtifactLayout} from './layout'

export function LayoutToggle() {
  const layout = useArtifactLayout((s) => s.layout)
  return (
    <Segmented<ArtifactLayout>
      aria-label="Layout"
      value={layout}
      onValueChange={setArtifactLayout}
      options={[
        {value: 'grid', label: <LayoutGrid aria-label="Cards" />, tip: 'Show the artifacts as cards'},
        {value: 'list', label: <List aria-label="List" />, tip: 'Show the artifacts as a compact list'},
      ]}
    />
  )
}
