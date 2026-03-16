import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useState } from 'react'
import Header from './components/Header'
import { setAuthToken } from './api/client'
import AdminLayout from './components/AdminLayout'
import DepositPage from './pages/DepositPage'
import StatusPage from './pages/StatusPage'
import FlowPage from './pages/FlowPage'
import QueuePage from './pages/QueuePage'
import LedgerPage from './pages/LedgerPage'
import InvestorLoginPage from './pages/InvestorLoginPage'
import InvestorDashboardPage from './pages/InvestorDashboardPage'
import RiskDashboardPage from './pages/RiskDashboardPage'

export interface DemoUser {
  id: string
  label: string
  role: 'investor' | 'operator' | 'apex_admin'
  correspondent?: string
}

export const DEMO_USERS: DemoUser[] = [
  { id: 'operator-alpha', label: 'Operator (Alpha)', role: 'operator', correspondent: 'ALPHA' },
  { id: 'operator-beta', label: 'Operator (Beta)', role: 'operator', correspondent: 'BETA' },
  { id: 'apex-admin', label: 'Apex Admin', role: 'apex_admin' },
]

export interface InvestorAccount {
  id: string
  code: string
  type: string
  name: string
}

function App() {
  const [currentUser, setCurrentUser] = useState<DemoUser>(DEMO_USERS[0])
  const [investorAccount, setInvestorAccount] = useState<InvestorAccount | null>(null)
  const location = useLocation()
  const isAdmin = location.pathname.startsWith('/admin')

  // Synchronously set the auth token every render — ensures child components
  // always fetch with the correct token, even on the very first mount.
  // Investor tokens must match the demo tokens in the backend auth middleware.
  if (isAdmin || !investorAccount) {
    setAuthToken(currentUser.id)
  } else {
    const investorToken = investorAccount.code.startsWith('BETA')
      ? 'investor-beta'
      : 'investor-alpha'
    setAuthToken(investorToken)
  }

  function handleInvestorLogin(account: InvestorAccount) {
    setInvestorAccount(account)
  }

  function handleInvestorLogout() {
    setInvestorAccount(null)
  }

  // Clear investor session when on admin routes (deferred to avoid render-phase setState)
  if (isAdmin && investorAccount) {
    // Schedule clearing after this render completes
    setTimeout(() => setInvestorAccount(null), 0)
  }

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <Header
        currentUser={currentUser}
        onUserChange={setCurrentUser}
        investorAccount={isAdmin ? null : investorAccount}
        onInvestorLogout={handleInvestorLogout}
      />
      <main style={{ flex: 1, background: '#f4f4f4' }}>
        <Routes>
          {/* Investor routes */}
          <Route path="/login" element={
            investorAccount
              ? <Navigate to="/dashboard" replace />
              : <InvestorLoginPage onLogin={handleInvestorLogin} />
          } />
          <Route path="/dashboard" element={
            investorAccount
              ? <InvestorDashboardPage account={investorAccount} />
              : <Navigate to="/login" replace />
          } />
          <Route path="/deposit" element={
            investorAccount
              ? <DepositPage accountCode={investorAccount.code} />
              : <Navigate to="/login" replace />
          } />
          <Route path="/status/:id" element={<StatusPage />} />

          {/* Admin/operator routes */}
          <Route path="/admin" element={<AdminLayout />}>
            <Route path="flow" element={<FlowPage />} />
            <Route path="queue" element={<QueuePage />} />
            <Route path="ledger" element={<LedgerPage />} />
            <Route path="risk" element={<RiskDashboardPage />} />
          </Route>

          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
