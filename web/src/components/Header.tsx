import { Link } from 'react-router-dom'
import { DEMO_USERS, type DemoUser, type InvestorAccount } from '../App'
import NotificationBell from './NotificationBell'

interface HeaderProps {
  currentUser: DemoUser
  onUserChange: (user: DemoUser) => void
  investorAccount: InvestorAccount | null
  onInvestorLogout: () => void
}

export default function Header({ currentUser, onUserChange, investorAccount, onInvestorLogout }: HeaderProps) {
  return (
    <header style={{
      background: '#1a1a2e',
      color: '#fff',
      padding: '0.75rem 1.5rem',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      flexWrap: 'wrap',
      gap: '0.5rem',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '1.5rem' }}>
        <Link to={investorAccount ? '/dashboard' : '/admin/flow'} style={{ color: '#fff', textDecoration: 'none', fontWeight: 'bold', fontSize: '1.1rem' }}>
          Apex Check Deposit
        </Link>
        <nav style={{ display: 'flex', gap: '1rem' }}>
          {investorAccount ? (
            <>
              <Link to="/dashboard" style={{ color: '#ccc', textDecoration: 'none' }}>Dashboard</Link>
              <Link to="/deposit" style={{ color: '#ccc', textDecoration: 'none' }}>Deposit</Link>
            </>
          ) : (
            <>
              <Link to="/login" style={{ color: '#ccc', textDecoration: 'none' }}>Investor</Link>
              <Link to="/admin/flow" style={{ color: '#ccc', textDecoration: 'none' }}>Admin</Link>
            </>
          )}
        </nav>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
        {investorAccount ? (
          <>
            <NotificationBell userKey={`investor:${investorAccount.code}`} />
            <span style={{ fontSize: '0.85rem', color: '#aaa' }}>
              {investorAccount.name}
            </span>
            <span style={{ fontSize: '0.8rem', color: '#666', fontFamily: 'monospace' }}>
              {investorAccount.code}
            </span>
            <button
              onClick={onInvestorLogout}
              style={{
                padding: '0.3rem 0.6rem',
                background: 'transparent',
                color: '#f44336',
                border: '1px solid #f44336',
                borderRadius: '4px',
                cursor: 'pointer',
                fontSize: '0.8rem',
              }}
            >
              Logout
            </button>
          </>
        ) : (
          <>
            <label htmlFor="role-switcher" style={{ fontSize: '0.85rem', color: '#aaa' }}>Role:</label>
            <select
              id="role-switcher"
              value={currentUser.id}
              onChange={(e) => {
                const user = DEMO_USERS.find(u => u.id === e.target.value)
                if (user) onUserChange(user)
              }}
              style={{
                padding: '0.35rem 0.5rem',
                borderRadius: '4px',
                border: '1px solid #444',
                background: '#16213e',
                color: '#fff',
                fontSize: '0.85rem',
              }}
            >
              {DEMO_USERS.map(user => (
                <option key={user.id} value={user.id}>{user.label}</option>
              ))}
            </select>
          </>
        )}
      </div>
    </header>
  )
}
