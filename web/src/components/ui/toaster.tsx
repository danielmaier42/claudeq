import {Toaster as Sonner} from 'sonner'
import {resolvedDark, useTheme} from '@/lib/theme'

export {toast} from 'sonner'

export function Toaster() {
  const dark = useTheme(resolvedDark)
  return (
    <Sonner
      theme={dark ? 'dark' : 'light'}
      position="bottom-right"
      offset={16}
      gap={8}
      toastOptions={{
        classNames: {
          toast:
            '!rounded-lg !border !border-border !bg-surface !text-foreground !shadow-md !text-[13px] !font-sans',
          description: '!text-muted-foreground',
          actionButton: '!bg-accent !text-accent-foreground',
        },
      }}
    />
  )
}
