import { useParams } from 'react-router-dom'

export default function StatusPage() {
  const { id } = useParams()
  return (
    <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '1rem' }}>Transfer Status</h1>
      <p style={{ color: '#666' }}>Status for transfer {id} — coming in TB1</p>
    </div>
  )
}
