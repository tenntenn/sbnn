import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { applyPageTitle } from './pageTitle'
import './styles.css'

// Before anything is drawn, so a tab restored in the background is already
// named even if React never gets around to rendering it.
applyPageTitle()

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
}
