import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { startURLState } from './urlState'
import './styles.css'

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
  // After the render is asked for, not after it has happened: the sections
  // arrive later either way, and urlState waits for them itself.
  startURLState()
}
