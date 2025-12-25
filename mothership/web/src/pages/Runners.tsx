import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import GlassCard from '../components/GlassCard'
import { EditIcon, CheckIcon, XIcon } from '../components/Icons'

interface GPUInfo {
  name: string
  memory_gb: number
  driver?: string
}

interface Runner {
  id: string
  device_id?: string
  name: string
  hostname: string
  os: string
  architecture: string
  status: string
  active_tasks: number
  max_concurrent_tasks: number
  last_heartbeat: string
  cpu_cores?: number
  memory_gb?: number
  disk_space_gb?: number
  gpu_info?: string
  public_ips?: string
}

export default function Runners() {
  const queryClient = useQueryClient()
  const [editingRunner, setEditingRunner] = useState<string | null>(null)
  const [newName, setNewName] = useState('')

  const { data: runners, isLoading } = useQuery<Runner[]>({
    queryKey: ['runners'],
    queryFn: async () => {
      const res = await axios.get('/api/v1/runners')
      return res.data
    },
    refetchInterval: 5000,
  })

  const renameMutation = useMutation({
    mutationFn: async ({ runnerId, name }: { runnerId: string; name: string }) => {
      await axios.patch(`/api/v1/runners/${runnerId}/rename`, { name })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['runners'] })
      setEditingRunner(null)
      setNewName('')
    },
  })

  const handleRename = (runnerId: string) => {
    if (newName.trim()) {
      renameMutation.mutate({ runnerId, name: newName.trim() })
    }
  }
  
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'idle':
        return 'text-green-400'
      case 'busy':
        return 'text-blue-400'
      case 'offline':
        return 'text-red-400'
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
  
  const parseGPUInfo = (gpuInfoStr?: string): GPUInfo[] => {
    if (!gpuInfoStr) return []
    try {
      return JSON.parse(gpuInfoStr)
    } catch {
      return []
    }
  }

  const parsePublicIPs = (ipsStr?: string): string[] => {
    if (!ipsStr) return []
    try {
      return JSON.parse(ipsStr)
    } catch {
      return []
    }
  }

  return (
    <div className="p-8">
      <h1 className="text-3xl font-bold mb-8">Runners</h1>
      
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {runners?.map((runner) => {
          const gpus = parseGPUInfo(runner.gpu_info)
          const publicIPs = parsePublicIPs(runner.public_ips)
          const isEditing = editingRunner === runner.id

          return (
            <GlassCard key={runner.id}>
              <div className="space-y-4">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    {isEditing ? (
                      <div className="flex gap-2">
                        <input
                          type="text"
                          value={newName || runner.name}
                          onChange={(e) => setNewName(e.target.value)}
                          className="bg-gray-800 text-white px-2 py-1 rounded text-sm flex-1"
                          autoFocus
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                              handleRename(runner.id)
                            } else if (e.key === 'Escape') {
                              setEditingRunner(null)
                              setNewName('')
                            }
                          }}
                        />
                        <button
                          onClick={() => handleRename(runner.id)}
                          className="px-2 py-1 bg-blue-600 text-white rounded text-xs hover:bg-blue-700"
                        >
                          <CheckIcon />
                        </button>
                        <button
                          onClick={() => {
                            setEditingRunner(null)
                            setNewName('')
                          }}
                          className="px-2 py-1 bg-gray-600 text-white rounded text-xs hover:bg-gray-700"
                        >
                          <XIcon />
                        </button>
                      </div>
                    ) : (
                      <>
                        <h3 className="text-xl font-semibold mb-1 flex items-center gap-2">
                          {runner.name}
                          <button
                            onClick={() => {
                              setEditingRunner(runner.id)
                              setNewName(runner.name)
                            }}
                            className="text-gray-400 hover:text-white"
                            title="Rename runner"
                          >
                            <EditIcon />
                          </button>
                        </h3>
                        {runner.device_id && (
                          <p className="text-xs text-gray-500 font-mono">ID: {runner.device_id.substring(0, 8)}...</p>
                        )}
                      </>
                    )}
                    <p className="text-sm text-gray-400">{runner.hostname}</p>
                  </div>
                </div>
                
                <div className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-gray-400">OS:</span>
                    <span>{runner.os}/{runner.architecture}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Status:</span>
                    <span className={getStatusColor(runner.status)}>{runner.status}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Tasks:</span>
                    <span>{runner.active_tasks}/{runner.max_concurrent_tasks}</span>
                  </div>
                  
                  {/* Resource Information */}
                  {runner.cpu_cores !== undefined && runner.cpu_cores > 0 && (
                    <div className="flex justify-between">
                      <span className="text-gray-400">CPU:</span>
                      <span>{runner.cpu_cores} cores</span>
                    </div>
                  )}
                  {runner.memory_gb !== undefined && runner.memory_gb > 0 && (
                    <div className="flex justify-between">
                      <span className="text-gray-400">RAM:</span>
                      <span>{runner.memory_gb.toFixed(1)} GB</span>
                    </div>
                  )}
                  {runner.disk_space_gb !== undefined && runner.disk_space_gb > 0 && (
                    <div className="flex justify-between">
                      <span className="text-gray-400">Disk:</span>
                      <span>{runner.disk_space_gb.toFixed(1)} GB available</span>
                    </div>
                  )}
                  {gpus.length > 0 && (
                    <div className="flex justify-between">
                      <span className="text-gray-400">GPU:</span>
                      <span className="text-right">
                        {gpus.map((gpu, idx) => (
                          <div key={idx} className="text-xs">
                            {gpu.name} ({gpu.memory_gb > 0 ? `${gpu.memory_gb.toFixed(1)} GB` : 'N/A'})
                          </div>
                        ))}
                      </span>
                    </div>
                  )}
                  {publicIPs.length > 0 && (
                    <div className="flex justify-between">
                      <span className="text-gray-400">Public IP:</span>
                      <span className="text-xs font-mono">{publicIPs.join(', ')}</span>
                    </div>
                  )}
                  
                  <div className="flex justify-between">
                    <span className="text-gray-400">Last Heartbeat:</span>
                    <span>{new Date(runner.last_heartbeat).toLocaleTimeString()}</span>
                  </div>
                </div>
              </div>
            </GlassCard>
          )
        })}
      </div>
      
      {runners?.length === 0 && (
        <GlassCard>
          <div className="text-center text-gray-400 py-8">No runners found</div>
        </GlassCard>
      )}
    </div>
  )
}

