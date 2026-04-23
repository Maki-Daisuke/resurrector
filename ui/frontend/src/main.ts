import './style.css'
import {
  AllCommunityModule,
  ModuleRegistry,
  provideGlobalGridOptions,
} from 'ag-grid-community'
import App from './App.svelte'

// Register all AG Grid Community modules once for the app lifetime, and keep
// using the legacy CSS theme (ag-theme-alpine) instead of the new Theming API.
ModuleRegistry.registerModules([AllCommunityModule])
provideGlobalGridOptions({ theme: 'legacy' })

const app = new App({
  target: document.getElementById('app')
})

export default app
