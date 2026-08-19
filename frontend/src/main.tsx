import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { AgentLogWindow } from './components/AgentLogWindow'

const isAgentLogWindow = new URLSearchParams(window.location.search).has('agent-log');

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    {isAgentLogWindow ? <AgentLogWindow /> : <App />}
  </React.StrictMode>,
)
