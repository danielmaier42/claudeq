import {OctagonAlert, RotateCcw} from 'lucide-react'
import {Component, type ErrorInfo, type ReactNode} from 'react'
import {Button} from '@/components/ui/button'

// ErrorBoundary catches a rendering error anywhere below it. Without it React
// unmounts the whole tree and the window goes blank; with it the user sees what
// broke and can reload, and the daemon keeps running either way.
export class ErrorBoundary extends Component<{children: ReactNode}, {error: Error | null}> {
  state = {error: null as Error | null}

  static getDerivedStateFromError(error: Error) {
    return {error}
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('DispatchQ UI crashed:', error, info.componentStack)
  }

  render() {
    const {error} = this.state
    if (!error) return this.props.children
    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="max-w-md text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-2xl bg-danger-soft text-danger">
            <OctagonAlert className="size-6" />
          </div>
          <h1 className="text-[15px] font-semibold">Something went wrong in the window</h1>
          <p className="mt-1 text-[13px] text-muted-foreground">
            Your tasks are safe: the daemon keeps running. Reload the window to continue.
          </p>
          <pre className="mt-4 max-h-40 overflow-auto rounded-md border border-border bg-surface-2 p-3 text-left font-mono text-[11.5px] whitespace-pre-wrap text-muted-foreground">
            {error.message}
          </pre>
          <Button variant="primary" className="mt-4" onClick={() => location.reload()}>
            <RotateCcw /> Reload
          </Button>
        </div>
      </div>
    )
  }
}
