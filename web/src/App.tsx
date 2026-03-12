import { Routes, Route, Navigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import Header from './components/Header'
import { setAuthToken } from './api/client'
import AdminLayout from './components/AdminLayout'
import DepositPage from './pages/DepositPage'
import StatusPage from './pages/StatusPage'
import FlowPage from './pages/FlowPage'
import QueuePage from './pages/QueuePage'
import LedgerPage from './pages/LedgerPage'

export interface DemoUser {
  id: string
  label: string
  role: 'investor' | 'operator' | 'apex_admin'
  correspondent?: string
}

export const DEMO_USERS: DemoUser[] = [
  { id: 'investor-alpha', label: 'Investor (Alpha)', role: 'investor', correspondent: 'ALPHA' },
  { id: 'investor-beta', label: 'Investor (Beta)', role: 'investor', correspondent: 'BETA' },
  { id: 'operator-alpha', label: 'Operator (Alpha)', role: 'operator', correspondent: 'ALPHA' },
  { id: 'operator-beta', label: 'Operator (Beta)', role: 'operator', correspondent: 'BETA' },
  { id: 'apex-admin', label: 'Apex Admin', role: 'apex_admin' },
]

function App() {
  const [currentUser, setCurrentUser] = useState<DemoUser>(DEMO_USERS[0])

  // Sync auth token whenever the demo user changes
  useEffect(() => {
    setAuthToken(currentUser.id)
  }, [currentUser])

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <Header currentUser={currentUser} onUserChange={setCurrentUser} />
      <main style={{ flex: 1 }}>
        <Routes>
          <Route path="/deposit" element={<DepositPage />} />
          <Route path="/status/:id" element={<StatusPage />} />
          <Route path="/admin" element={<AdminLayout />}>
            <Route path="flow" element={<FlowPage />} />
            <Route path="queue" element={<QueuePage />} />
            <Route path="ledger" element={<LedgerPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/deposit" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
