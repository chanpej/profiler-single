import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { MemoryWindow } from './components/MemoryWindow'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    {new URLSearchParams(location.search).get('window') === 'memory' ? <MemoryWindow /> : <App />}
  </React.StrictMode>,
)
