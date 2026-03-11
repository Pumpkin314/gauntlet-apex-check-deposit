import { NavLink, Outlet } from 'react-router-dom'

const navLinkStyle = (isActive: boolean) => ({
  display: 'block',
  padding: '0.6rem 1rem',
  textDecoration: 'none',
  color: isActive ? '#fff' : '#ccc',
  background: isActive ? '#2a2a4a' : 'transparent',
  borderRadius: '4px',
  fontSize: '0.9rem',
})

export default function AdminLayout() {
  return (
    <div className="admin-layout">
      <aside className="admin-sidebar">
        <nav style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
          <NavLink to="/admin/flow" style={({ isActive }) => navLinkStyle(isActive)}>
            Flow Dashboard
          </NavLink>
          <NavLink to="/admin/queue" style={({ isActive }) => navLinkStyle(isActive)}>
            Review Queue
          </NavLink>
          <NavLink to="/admin/ledger" style={({ isActive }) => navLinkStyle(isActive)}>
            Ledger
          </NavLink>
        </nav>
      </aside>
      <div className="admin-content">
        <Outlet />
      </div>
    </div>
  )
}
