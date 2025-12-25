import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import GlassCard from '../components/GlassCard'

interface Job {
  id: string
  name: string
  status: string
  type: string
  priority: number
  created_at: string
}

export default function Jobs() {
  const queryClient = useQueryClient()
  
  const { data: jobsData, isLoading } = useQuery<{ jobs: Job[], total: number }>({
    queryKey: ['jobs'],
    queryFn: async () => {
      const res = await axios.get('/api/v1/jobs')
      return res.data
    },
    refetchInterval: 5000,
  })
  
  const pauseMutation = useMutation({
    mutationFn: async (jobId: string) => {
      await axios.post(`/api/v1/jobs/${jobId}/pause`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
  
  const resumeMutation = useMutation({
    mutationFn: async (jobId: string) => {
      await axios.post(`/api/v1/jobs/${jobId}/resume`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
  
  const cancelMutation = useMutation({
    mutationFn: async (jobId: string) => {
      await axios.post(`/api/v1/jobs/${jobId}/cancel`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
  
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'pending':
        return 'text-yellow-400'
      case 'running':
        return 'text-blue-400'
      case 'completed':
        return 'text-green-400'
      case 'failed':
        return 'text-red-400'
      case 'paused':
        return 'text-gray-400'
      default:
        return 'text-gray-400'
    }
  }
  
  if (isLoading) {
    return (
      <div className="p-8">
        <div className="text-white">Loading...</div>
      </div>
    )
  }
  
  return (
    <div className="p-8">
      <div className="flex justify-between items-center mb-8">
        <h1 className="text-3xl font-bold">Jobs</h1>
        <button className="bg-white text-black px-4 py-2 rounded-lg font-medium hover:bg-gray-200 transition-colors">
          Create Job
        </button>
      </div>
      
      <div className="space-y-4">
        {jobsData?.jobs?.map((job) => (
          <GlassCard key={job.id}>
            <div className="flex justify-between items-start">
              <div>
                <h3 className="text-xl font-semibold mb-2">{job.name}</h3>
                <div className="flex items-center space-x-4 text-sm text-gray-400">
                  <span>Type: {job.type}</span>
                  <span>Priority: {job.priority}</span>
                  <span className={getStatusColor(job.status)}>Status: {job.status}</span>
                </div>
              </div>
              <div className="flex space-x-2">
                {job.status === 'running' && (
                  <button
                    onClick={() => pauseMutation.mutate(job.id)}
                    className="px-3 py-1 bg-yellow-500/20 text-yellow-400 rounded hover:bg-yellow-500/30 transition-colors"
                  >
                    Pause
                  </button>
                )}
                {job.status === 'paused' && (
                  <button
                    onClick={() => resumeMutation.mutate(job.id)}
                    className="px-3 py-1 bg-blue-500/20 text-blue-400 rounded hover:bg-blue-500/30 transition-colors"
                  >
                    Resume
                  </button>
                )}
                {(job.status === 'pending' || job.status === 'running' || job.status === 'paused') && (
                  <button
                    onClick={() => cancelMutation.mutate(job.id)}
                    className="px-3 py-1 bg-red-500/20 text-red-400 rounded hover:bg-red-500/30 transition-colors"
                  >
                    Cancel
                  </button>
                )}
              </div>
            </div>
          </GlassCard>
        ))}
      </div>
      
      {jobsData?.jobs?.length === 0 && (
        <GlassCard>
          <div className="text-center text-gray-400 py-8">No jobs found</div>
        </GlassCard>
      )}
    </div>
  )
}

