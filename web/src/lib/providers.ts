import {useModels, useProviders, useSettings} from './queries'
import type {Model, Provider} from './types'

// useBeta reports whether Settings → System → Beta features is on. It only
// decides what the app offers; the API and the scheduler accept everything.
export function useBeta(): boolean {
  return !!useSettings().data?.beta_features
}

export const providerName = (p: Pick<Provider, 'id' | 'name'>) => p.name || p.id

export function defaultProvider(providers: Provider[] | undefined): Provider | undefined {
  return providers?.find((p) => p.default)
}

// modelLabel turns a model id into what the catalog calls it, falling back to
// the id: a task may name a model discovery never listed.
export function modelLabel(models: Model[] | undefined, id: string): string {
  return models?.find((m) => m.id === id)?.label ?? id
}

// useModelLabel resolves model ids against the global catalog.
export function useModelLabel() {
  const models = useModels().data
  return (id: string) => modelLabel(models, id)
}

export function useProviderList() {
  return useProviders().data ?? []
}
