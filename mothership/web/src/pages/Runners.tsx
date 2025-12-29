import { useState, useRef, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import axios from 'axios'
import GlassCard from '../components/GlassCard'
import Modal from '../components/Modal'
import { EditIcon, CheckIcon, XIcon, TrashIcon, CPUIcon, RAMIcon, DiskIcon, IPIcon, OSIcon, GPUIcon, MoreIcon, MonitorIcon } from '../components/Icons'

interface GPUInfo {
  name: string
  memory_gb: number
  driver?: string
}

interface RuntimeConfig {
  name: string
  path?: string
  url?: string
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
  cpu_model?: string
  cpu_frequency_mhz?: number
  memory_gb?: number
  disk_space_gb?: number
  total_disk_space_gb?: number
  os_version?: string
  gpu_info?: string
  public_ips?: string
  screen_monitoring_enabled?: boolean
  screen_quality?: number
  screen_fps?: number
  runtimes?: string | RuntimeConfig[] // JSON string or parsed array
}

export default function Runners() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [editingRunner, setEditingRunner] = useState<string | null>(null)
  const [newName, setNewName] = useState('')
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  const menuRefs = useRef<{ [key: string]: HTMLDivElement | null }>({})
  const [deleteModal, setDeleteModal] = useState<{ isOpen: boolean; runnerId: string; runnerName: string }>({
    isOpen: false,
    runnerId: '',
    runnerName: '',
  })

  const { data: runners, isLoading, error } = useQuery<Runner[]>({
    queryKey: ['runners'],
    queryFn: async () => {
      const res = await axios.get('/api/v1/runners')
      return res.data
    },
    refetchInterval: 5000,
    retry: 3,
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

  const deleteMutation = useMutation({
    mutationFn: async (runnerId: string) => {
      await axios.delete(`/api/v1/runners/${runnerId}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['runners'] })
    },
  })

  // Close menu when clicking outside - MUST be called before any early returns
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (openMenuId) {
        const menuElement = menuRefs.current[openMenuId]
        if (menuElement && !menuElement.contains(event.target as Node)) {
          setOpenMenuId(null)
        }
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [openMenuId])

  const handleRename = (runnerId: string) => {
    if (newName.trim()) {
      renameMutation.mutate({ runnerId, name: newName.trim() })
    }
  }

  const handleDelete = (runnerId: string, runnerName: string) => {
    setDeleteModal({ isOpen: true, runnerId, runnerName })
  }

  const confirmDelete = () => {
    deleteMutation.mutate(deleteModal.runnerId)
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

  if (error) {
    return (
      <div className="p-8">
        <div className="text-red-400">Error loading runners: {error instanceof Error ? error.message : 'Unknown error'}</div>
        <div className="text-gray-400 mt-2">Check browser console for details</div>
      </div>
    )
  }
  
  const parsePublicIPs = (ipsStr?: string | null): string[] => {
    if (!ipsStr || ipsStr === 'null' || ipsStr === 'undefined') return []
    try {
      const parsed = JSON.parse(ipsStr)
      // Handle case where JSON.parse returns null
      if (parsed === null || parsed === undefined) return []
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }

  const parseGPUInfo = (gpuInfoStr?: string | null): GPUInfo[] => {
    if (!gpuInfoStr || gpuInfoStr === 'null' || gpuInfoStr === 'undefined') return []
    try {
      const parsed = JSON.parse(gpuInfoStr)
      // Handle case where JSON.parse returns null
      if (parsed === null || parsed === undefined) return []
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }

  const parseRuntimes = (runtimesStr?: string | RuntimeConfig[] | null): RuntimeConfig[] => {
    if (!runtimesStr || runtimesStr === 'null' || runtimesStr === 'undefined') return []
    // If it's already an array, return it
    if (Array.isArray(runtimesStr)) return runtimesStr
    // If it's a string, try to parse it
    if (typeof runtimesStr === 'string') {
      try {
        const parsed = JSON.parse(runtimesStr)
        if (parsed === null || parsed === undefined) return []
        return Array.isArray(parsed) ? parsed : []
      } catch {
        return []
      }
    }
    return []
  }

  // Formatting helper functions
  const formatGB = (gb: number): string => {
    if (gb === 0) return '0 GB'
    if (gb < 1) return `${(gb * 1024).toFixed(0)} MB`
    return `${gb.toFixed(1)} GB`
  }

  const formatFrequency = (mhz: number): string => {
    if (mhz === 0) return ''
    if (mhz >= 1000) {
      return `${(mhz / 1000).toFixed(2)} GHz`
    }
    return `${mhz} MHz`
  }

  const filterIPv4 = (ips: string[] | null | undefined): string[] => {
    if (!ips || !Array.isArray(ips) || ips === null) return []
    return ips.filter((ip): ip is string => {
      // Simple IPv4 check (contains dots and no colons)
      return !!ip && typeof ip === 'string' && ip.includes('.') && !ip.includes(':')
    })
  }

  const formatOSVersion = (os: string, version?: string): string => {
    if (!version) return os
    return version
  }

  const getStatusBadgeColor = (status: string): string => {
    switch (status) {
      case 'idle':
        return 'bg-green-500'
      case 'busy':
        return 'bg-blue-500'
      case 'offline':
        return 'bg-red-500'
      default:
        return 'bg-gray-500'
    }
  }

  return (
    <div className="p-8">
      <h1 className="text-3xl font-bold mb-8">Runners</h1>
      
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {(runners || []).map((runner) => {
          const publicIPs = parsePublicIPs(runner.public_ips) || []
          const ipv4Addresses = filterIPv4(publicIPs) || []
          const gpus = parseGPUInfo(runner.gpu_info) || []
          const isEditing = editingRunner === runner.id

          return (
            <GlassCard key={runner.id} className="p-4">
              {/* Header Row: Status Badge | Name | Status + Actions */}
              <div className="flex items-center justify-between mb-4">
                {/* Left: Status Badge + Screen Monitoring */}
                <div className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${getStatusBadgeColor(runner.status)}`}></div>
                  <span className={`text-xs font-medium capitalize ${getStatusColor(runner.status)}`}>
                    {runner.status === 'offline' ? 'Offline' : runner.status}
                  </span>
                  {/* Screen Monitoring Indicator */}
                  <div className="flex items-center gap-1">
                    <div 
                      className={`w-2 h-2 rounded-full ${
                        runner.screen_monitoring_enabled 
                          ? 'bg-green-400' 
                          : 'bg-gray-500'
                      }`}
                    ></div>
                    <span className={`text-xs ${
                      runner.screen_monitoring_enabled 
                        ? 'text-green-400' 
                        : 'text-gray-500'
                    }`}>
                      {runner.screen_monitoring_enabled ? 'Monitor' : 'No Monitor'}
                    </span>
                  </div>
                </div>

                {/* Center: Solder Name */}
                <div className="flex-1 mx-4">
                  {isEditing ? (
                    <div className="flex gap-2 items-center">
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
                    <div className="flex items-center gap-2 justify-center">
                      <h3 className="text-lg font-semibold">{runner.name}</h3>
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
                    </div>
                  )}
                </div>

                {/* Right: Actions */}
                <div className="flex items-center gap-2 relative">
                  <span className="text-xs text-gray-400">{runner.active_tasks}/{runner.max_concurrent_tasks}</span>
                  
                  {/* Three-dot menu */}
                  <div className="relative" ref={(el) => { menuRefs.current[runner.id] = el }}>
                    <button
                      onClick={() => setOpenMenuId(openMenuId === runner.id ? null : runner.id)}
                      className="text-gray-400 hover:text-white p-1"
                      title="More options"
                    >
                      <MoreIcon />
                    </button>
                    
                    {openMenuId === runner.id && (
                      <div className="absolute right-0 mt-1 w-40 bg-gray-800 border border-gray-700 rounded-lg shadow-lg z-10">
                        <button
                          onClick={() => {
                            navigate(`/runners/${runner.id}/monitor`)
                            setOpenMenuId(null)
                          }}
                          disabled={!runner.screen_monitoring_enabled}
                          className={`w-full text-left px-4 py-2 text-sm ${
                            runner.screen_monitoring_enabled
                              ? 'text-white hover:bg-gray-700'
                              : 'text-gray-500 cursor-not-allowed'
                          } flex items-center gap-2`}
                        >
                          <MonitorIcon />
                          Monitor
                        </button>
                      </div>
                    )}
                  </div>
                  
                  <button
                    onClick={() => handleDelete(runner.id, runner.name)}
                    className="text-red-400 hover:text-red-300"
                    title="Delete runner"
                    disabled={deleteMutation.isPending}
                  >
                    <TrashIcon />
                  </button>
                </div>
              </div>

              {/* Resources Section: Compact Grid with Icons */}
              <div className="grid grid-cols-2 gap-3 text-sm">
                {/* IPv4 */}
                {ipv4Addresses.length > 0 && (
                  <div className="flex items-center gap-2">
                    <IPIcon />
                    <div className="flex-1 min-w-0">
                      <div className="text-gray-400 text-xs">IPv4</div>
                      <div className="text-white font-mono text-xs truncate">{ipv4Addresses[0]}</div>
                    </div>
                  </div>
                )}

                {/* CPU */}
                {(runner.cpu_model || runner.cpu_cores) && (
                  <div className="flex items-center gap-2">
                    <CPUIcon />
                    <div className="flex-1 min-w-0">
                      <div className="text-gray-400 text-xs">CPU</div>
                      <div className="text-white text-xs">
                        {runner.cpu_model ? (
                          <>
                            {runner.cpu_model}
                            {runner.cpu_frequency_mhz && runner.cpu_frequency_mhz > 0 && (
                              <> @ {formatFrequency(runner.cpu_frequency_mhz)}</>
                            )}
                            {runner.cpu_cores && runner.cpu_cores > 0 && (
                              <> ({runner.cpu_cores} cores)</>
                            )}
                          </>
                        ) : runner.cpu_cores && runner.cpu_cores > 0 ? (
                          `${runner.cpu_cores} cores`
                        ) : (
                          <span className="text-gray-500">Not detected</span>
                        )}
                      </div>
                    </div>
                  </div>
                )}

                {/* RAM */}
                {runner.memory_gb !== undefined && (
                  <div className="flex items-center gap-2">
                    <RAMIcon />
                    <div className="flex-1 min-w-0">
                      <div className="text-gray-400 text-xs">RAM</div>
                      <div className="text-white text-xs">
                        {runner.memory_gb > 0 ? formatGB(runner.memory_gb) : <span className="text-gray-500">Not detected</span>}
                      </div>
                    </div>
                  </div>
                )}

                {/* Disk */}
                {(runner.disk_space_gb !== undefined || runner.total_disk_space_gb !== undefined) && (
                  <div className="flex items-center gap-2">
                    <DiskIcon />
                    <div className="flex-1 min-w-0">
                      <div className="text-gray-400 text-xs">Disk</div>
                      <div className="text-white text-xs">
                        {runner.disk_space_gb !== undefined && runner.total_disk_space_gb !== undefined && runner.disk_space_gb > 0 && runner.total_disk_space_gb > 0 ? (
                          <>
                            {formatGB(runner.disk_space_gb)} / {formatGB(runner.total_disk_space_gb)} remaining
                          </>
                        ) : runner.disk_space_gb !== undefined && runner.disk_space_gb > 0 ? (
                          <>{formatGB(runner.disk_space_gb)} remaining</>
                        ) : (
                          <span className="text-gray-500">Not detected</span>
                        )}
                      </div>
                    </div>
                  </div>
                )}

                {/* OS Version */}
                {(runner.os_version || runner.os) && (
                  <div className="flex items-center gap-2">
                    <OSIcon />
                    <div className="flex-1 min-w-0">
                      <div className="text-gray-400 text-xs">OS</div>
                      <div className="text-white text-xs">
                        {runner.os_version || formatOSVersion(runner.os, undefined)}
                      </div>
                    </div>
                  </div>
                )}

                {/* GPU */}
                <div className="flex items-center gap-2">
                  <GPUIcon />
                  <div className="flex-1 min-w-0">
                    <div className="text-gray-400 text-xs">GPU</div>
                    <div className="text-white text-xs">
                      {gpus.length > 0 ? (
                        gpus.map((gpu, idx) => (
                          <div key={idx} className="truncate">
                            {gpu.name}
                            {gpu.memory_gb > 0 && (
                              <> ({formatGB(gpu.memory_gb)} vRAM)</>
                            )}
                          </div>
                        ))
                      ) : (
                        <span className="text-gray-500">N/A</span>
                      )}
                    </div>
                  </div>
                </div>
              </div>

              {/* Runtimes Section */}
              {(() => {
                const runtimes = parseRuntimes(runner.runtimes)
                return runtimes.length > 0 && (
                  <div className="mt-3 pt-3 border-t border-gray-700">
                    <div className="text-gray-400 text-xs mb-2">Runtimes</div>
                    <div className="flex flex-wrap gap-2">
                      {runtimes.map((runtime, idx) => (
                        <div
                          key={idx}
                          className="px-2 py-1 bg-blue-600/20 border border-blue-500/30 rounded text-xs text-blue-300"
                          title={runtime.path || runtime.url || runtime.name}
                        >
                          {runtime.name}
                        </div>
                      ))}
                    </div>
                  </div>
                )
              })()}

              {/* Additional Info */}
              <div className="mt-3 pt-3 border-t border-gray-700 text-xs text-gray-500">
                <div className="flex justify-between">
                  <span>Hostname: {runner.hostname}</span>
                  <span>{new Date(runner.last_heartbeat).toLocaleTimeString()}</span>
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

      <Modal
        isOpen={deleteModal.isOpen}
        onClose={() => setDeleteModal({ isOpen: false, runnerId: '', runnerName: '' })}
        title="Delete Runner"
        onConfirm={confirmDelete}
        confirmText="Delete"
        danger
      >
        <p className="text-gray-300">
          Are you sure you want to delete runner <strong className="text-white">"{deleteModal.runnerName}"</strong>?
          This action cannot be undone.
        </p>
      </Modal>
    </div>
  )
}

