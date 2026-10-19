import {QueryClientProvider} from '@tanstack/react-query'
import {StrictMode} from 'react'
import {createRoot} from 'react-dom/client'
import {App} from './app/App'
import {ErrorBoundary} from './app/ErrorBoundary'
import {installNativeBridge} from './app/native-bridge'
import {queryClient} from './lib/queries'
import './styles.css'

// The UI is a React app served by the daemon from internal/api/web (built from
// web/ by Vite). Layout:
//
//   src/components/ui      design-system primitives (Radix + Tailwind)
//   src/components/layout  the frame views are drawn in
//   src/lib                api client, queries, theme, formatting, types
//   src/app                shell: sidebar, navigation, command palette
//   src/features/<name>    one folder per view or sheet, owning its UI and logic
installNativeBridge()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </QueryClientProvider>
  </StrictMode>,
)
