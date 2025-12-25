import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import GlassCard from '../components/GlassCard'
import Modal from '../components/Modal'

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
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [jobForm, setJobForm] = useState({
    name: '',
    description: '',
    type: 'shell',
    command: '',
    args: '',
    env: '',
    working_directory: '',
    timeout_seconds: 0,
    max_retries: 0,
    priority: 1,
  })
  
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

  const createJobMutation = useMutation({
    mutationFn: async (jobData: any) => {
      const payload: any = {
        name: jobData.name,
        description: jobData.description || '',
        type: jobData.type || 'shell',
        command: jobData.command,
        priority: parseInt(jobData.priority) || 1,
      }
      
      // Parse args - can be JSON array string or comma-separated
      if (jobData.args && jobData.args.trim()) {
        try {
          const parsed = JSON.parse(jobData.args)
          payload.args = parsed // Send as array, backend will convert to JSON string
        } catch {
          // Not valid JSON, treat as comma-separated
          const argsArray = jobData.args.split(',').map((a: string) => a.trim()).filter((a: string) => a)
          payload.args = argsArray
        }
      }
      
      // Parse env - can be JSON object string or KEY=VALUE format
      if (jobData.env && jobData.env.trim()) {
        try {
          const parsed = JSON.parse(jobData.env)
          payload.env = parsed // Send as object, backend will convert to JSON string
        } catch {
          // Not valid JSON, parse as KEY=VALUE lines
          const envObj: Record<string, string> = {}
          jobData.env.split('\n').forEach((line: string) => {
            const trimmed = line.trim()
            if (trimmed) {
              const [key, ...valueParts] = trimmed.split('=')
              if (key && valueParts.length > 0) {
                envObj[key.trim()] = valueParts.join('=').trim()
              }
            }
          })
          payload.env = envObj
        }
      }
      
      if (jobData.working_directory) payload.working_directory = jobData.working_directory
      if (jobData.timeout_seconds) payload.timeout_seconds = parseInt(jobData.timeout_seconds) || 0
      if (jobData.max_retries) payload.max_retries = parseInt(jobData.max_retries) || 0
      
      const res = await axios.post('/api/v1/jobs', payload)
      return res.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['jobs'] })
      setShowCreateModal(false)
      setJobForm({
        name: '',
        description: '',
        type: 'shell',
        command: '',
        args: '',
        env: '',
        working_directory: '',
        timeout_seconds: 0,
        max_retries: 0,
        priority: 1,
      })
    },
  })

  const handleCreateJob = () => {
    if (!jobForm.name || !jobForm.command) {
      alert('Name and command are required')
      return
    }
    createJobMutation.mutate(jobForm)
  }
  
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
        <button 
          onClick={() => setShowCreateModal(true)}
          className="bg-white text-black px-4 py-2 rounded-lg font-medium hover:bg-gray-200 transition-colors"
        >
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

      <Modal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        title="Create New Job"
        onConfirm={handleCreateJob}
        confirmText="Create"
        showCancel={true}
      >
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">Job Name *</label>
            <input
              type="text"
              value={jobForm.name}
              onChange={(e) => setJobForm({ ...jobForm, name: e.target.value })}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
              placeholder="My Job"
            />
          </div>
          
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">Description</label>
            <input
              type="text"
              value={jobForm.description}
              onChange={(e) => setJobForm({ ...jobForm, description: e.target.value })}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
              placeholder="Job description"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">Type</label>
            <select
              value={jobForm.type}
              onChange={(e) => setJobForm({ ...jobForm, type: e.target.value })}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
            >
              <option value="shell">Shell</option>
              <option value="binary">Binary</option>
              <option value="docker">Docker</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">Command *</label>
            <textarea
              value={jobForm.command}
              onChange={(e) => setJobForm({ ...jobForm, command: e.target.value })}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
              rows={3}
              placeholder="echo 'Hello World'"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">Arguments (JSON array or comma-separated)</label>
            <input
              type="text"
              value={jobForm.args}
              onChange={(e) => setJobForm({ ...jobForm, args: e.target.value })}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
              placeholder='["arg1", "arg2"] or arg1, arg2'
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">Environment Variables (JSON object or KEY=VALUE per line)</label>
            <textarea
              value={jobForm.env}
              onChange={(e) => setJobForm({ ...jobForm, env: e.target.value })}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
              rows={3}
              placeholder='{"KEY": "value"} or KEY=value'
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-1">Working Directory</label>
            <input
              type="text"
              value={jobForm.working_directory}
              onChange={(e) => setJobForm({ ...jobForm, working_directory: e.target.value })}
              className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
              placeholder="/tmp/work"
            />
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">Priority</label>
              <select
                value={jobForm.priority}
                onChange={(e) => setJobForm({ ...jobForm, priority: parseInt(e.target.value) })}
                className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
              >
                <option value={0}>Low</option>
                <option value={1}>Normal</option>
                <option value={2}>High</option>
                <option value={3}>Urgent</option>
              </select>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">Timeout (seconds)</label>
              <input
                type="number"
                value={jobForm.timeout_seconds}
                onChange={(e) => setJobForm({ ...jobForm, timeout_seconds: parseInt(e.target.value) || 0 })}
                className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
                placeholder="0"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">Max Retries</label>
              <input
                type="number"
                value={jobForm.max_retries}
                onChange={(e) => setJobForm({ ...jobForm, max_retries: parseInt(e.target.value) || 0 })}
                className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
                placeholder="0"
              />
            </div>
          </div>
        </div>
      </Modal>
    </div>
  )
}

