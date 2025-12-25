import { useQuery } from '@tanstack/react-query'
import axios from 'axios'
import GlassCard from '../components/GlassCard'
import BentoGrid from '../components/BentoGrid'

interface Stats {
  jobs: Record<string, number>
  tasks: Record<string, number>
  runners: number
}

export default function Dashboard() {
  const { data: stats, isLoading } = useQuery<Stats>({
    queryKey: ['stats'],
    queryFn: async () => {
      const res = await axios.get('/api/v1/stats')
      return res.data
    },
    refetchInterval: 5000,
  })
  
  if (isLoading) {
    return (
      <div className="p-8">
        <div className="text-white">Loading...</div>
      </div>
    )
  }
  
  return (
    <div className="p-8">
      <h1 className="text-3xl font-bold mb-8">Dashboard</h1>
      
      <BentoGrid>
        <GlassCard colSpan={2}>
          <h2 className="text-xl font-semibold mb-4">Jobs</h2>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <div className="text-3xl font-bold">{stats?.jobs?.pending || 0}</div>
              <div className="text-sm text-gray-400">Pending</div>
            </div>
            <div>
              <div className="text-3xl font-bold">{stats?.jobs?.running || 0}</div>
              <div className="text-sm text-gray-400">Running</div>
            </div>
            <div>
              <div className="text-3xl font-bold">{stats?.jobs?.completed || 0}</div>
              <div className="text-sm text-gray-400">Completed</div>
            </div>
            <div>
              <div className="text-3xl font-bold">{stats?.jobs?.failed || 0}</div>
              <div className="text-sm text-gray-400">Failed</div>
            </div>
          </div>
        </GlassCard>
        
        <GlassCard>
          <h2 className="text-xl font-semibold mb-4">Runners</h2>
          <div className="text-4xl font-bold">{stats?.runners || 0}</div>
          <div className="text-sm text-gray-400 mt-2">Active runners</div>
        </GlassCard>
        
        <GlassCard>
          <h2 className="text-xl font-semibold mb-4">Tasks</h2>
          <div className="space-y-2">
            <div>
              <div className="text-2xl font-bold">{stats?.tasks?.pending || 0}</div>
              <div className="text-xs text-gray-400">Pending</div>
            </div>
            <div>
              <div className="text-2xl font-bold">{stats?.tasks?.running || 0}</div>
              <div className="text-xs text-gray-400">Running</div>
            </div>
            <div>
              <div className="text-2xl font-bold">{stats?.tasks?.completed || 0}</div>
              <div className="text-xs text-gray-400">Completed</div>
            </div>
          </div>
        </GlassCard>
      </BentoGrid>
    </div>
  )
}

