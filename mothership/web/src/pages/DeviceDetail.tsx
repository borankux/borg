import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import GlassCard from '../components/GlassCard'
import { CPUIcon, RAMIcon, DiskIcon, IPIcon, OSIcon, GPUIcon, MonitorIcon } from '../components/Icons'

interface Runner {
  id: string
  name: string
  hostname: string
  os: string
  architecture: string
  status: string
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
  selected_screen_index?: number
}

interface ScreenInfo {
  index: number
  name?: string
  width: number
  height: number
  is_primary?: boolean
}

interface ScreenFrameMessage {
  type: string
  data: string // base64 data URL
  timestamp: number
}

export default function DeviceDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [currentFrame, setCurrentFrame] = useState<string | null>(null)
  const [connectionStatus, setConnectionStatus] = useState<'connecting' | 'connected' | 'disconnected' | 'error'>('disconnected')
  const [showDeviceInfo, setShowDeviceInfo] = useState<boolean>(false)
  const [quality, setQuality] = useState<number>(60)
  const [fps, setFps] = useState<number>(2.0)
  const [selectedScreenIndex, setSelectedScreenIndex] = useState<number>(0)
  const [availableScreens, setAvailableScreens] = useState<ScreenInfo[]>([])
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimeoutRef = useRef<number | null>(null)
  const queryClient = useQueryClient()

  const { data: runner, isLoading: runnerLoading } = useQuery<Runner>({
    queryKey: ['runner', id],
    queryFn: async () => {
      const res = await axios.get(`/api/v1/runners/${id}`)
      return res.data
    },
    enabled: !!id,
    refetchInterval: 5000,
  })

  // Initialize quality, fps, and screen index from runner data
  useEffect(() => {
    if (runner) {
      setQuality(runner.screen_quality || 60)
      setFps(runner.screen_fps || 2.0)
      setSelectedScreenIndex(runner.selected_screen_index || 0)
    }
  }, [runner])

  // Fetch available screens
  const { data: screensData } = useQuery<{ screens: ScreenInfo[] }>({
    queryKey: ['screens', id],
    queryFn: async () => {
      const res = await axios.get(`/api/v1/runners/${id}/screens`)
      return res.data
    },
    enabled: !!id && !!runner?.screen_monitoring_enabled,
    refetchInterval: 30000, // Refresh every 30 seconds
  })

  useEffect(() => {
    if (screensData?.screens) {
      setAvailableScreens(screensData.screens)
    }
  }, [screensData])

  // Mutation to update screen settings
  const updateSettingsMutation = useMutation({
    mutationFn: async (settings: { quality: number; fps: number; screen_index?: number }) => {
      try {
        const res = await axios.patch(`/api/v1/runners/${id}/screen-settings`, settings)
        return res.data
      } catch (error) {
        console.error('Failed to update screen settings:', error)
        if (axios.isAxiosError(error)) {
          console.error('Response:', error.response?.data)
        }
        throw error
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['runner', id] })
    },
  })

  // WebSocket connection for screen streaming
  useEffect(() => {
    if (!id || !runner?.screen_monitoring_enabled) {
      return
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/ws/screen/${id}`
    
    const connect = () => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        return
      }

      setConnectionStatus('connecting')
      const ws = new WebSocket(wsUrl)
      ws.binaryType = 'blob' // Handle binary frames as Blob
      wsRef.current = ws

      ws.onopen = () => {
        setConnectionStatus('connected')
        if (reconnectTimeoutRef.current) {
          clearTimeout(reconnectTimeoutRef.current)
          reconnectTimeoutRef.current = null
        }
      }

      ws.onmessage = (event) => {
        try {
          // Handle binary messages (JPEG frames)
          if (event.data instanceof Blob) {
            const reader = new FileReader()
            reader.onloadend = () => {
              if (reader.result) {
                setCurrentFrame(reader.result as string)
              }
            }
            reader.readAsDataURL(event.data)
          } else if (event.data instanceof ArrayBuffer) {
            // Handle ArrayBuffer
            const blob = new Blob([event.data], { type: 'image/jpeg' })
            const reader = new FileReader()
            reader.onloadend = () => {
              if (reader.result) {
                setCurrentFrame(reader.result as string)
              }
            }
            reader.readAsDataURL(blob)
          } else {
            // Fallback: try to parse as JSON (for backward compatibility)
            try {
              const message: ScreenFrameMessage = JSON.parse(event.data)
              if (message.type === 'frame' && message.data) {
                setCurrentFrame(message.data)
              }
            } catch (jsonErr) {
              console.error('Failed to parse WebSocket message:', jsonErr)
            }
          }
        } catch (err) {
          console.error('Failed to process WebSocket message:', err)
        }
      }

      ws.onerror = () => {
        setConnectionStatus('error')
      }

      ws.onclose = () => {
        setConnectionStatus('disconnected')
        wsRef.current = null
        
        // Attempt to reconnect after 3 seconds
        if (runner?.screen_monitoring_enabled) {
          reconnectTimeoutRef.current = setTimeout(() => {
            connect()
          }, 3000)
        }
      }
    }

    connect()

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current)
      }
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [id, runner?.screen_monitoring_enabled])

  const parsePublicIPs = (ipsStr?: string | null): string[] => {
    if (!ipsStr || ipsStr === 'null' || ipsStr === 'undefined') return []
    try {
      const parsed = JSON.parse(ipsStr)
      if (parsed === null || parsed === undefined) return []
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }

  const parseGPUInfo = (gpuInfoStr?: string | null): Array<{ name: string; memory_gb: number }> => {
    if (!gpuInfoStr || gpuInfoStr === 'null' || gpuInfoStr === 'undefined') return []
    try {
      const parsed = JSON.parse(gpuInfoStr)
      if (parsed === null || parsed === undefined) return []
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }

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

  const filterIPv4 = (ips: string[]): string[] => {
    if (!ips || !Array.isArray(ips)) return []
    return ips.filter((ip): ip is string => {
      return !!ip && typeof ip === 'string' && ip.includes('.') && !ip.includes(':')
    })
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

  const getConnectionStatusColor = () => {
    switch (connectionStatus) {
      case 'connected':
        return 'text-green-400'
      case 'connecting':
        return 'text-yellow-400'
      case 'error':
        return 'text-red-400'
      default:
        return 'text-gray-400'
    }
  }

  const getConnectionStatusText = () => {
    switch (connectionStatus) {
      case 'connected':
        return 'Streaming...'
      case 'connecting':
        return 'Connecting...'
      case 'error':
        return 'Connection Error'
      default:
        return 'Disconnected'
    }
  }

  if (runnerLoading) {
    return (
      <div className="p-8">
        <div className="text-white">Loading...</div>
      </div>
    )
  }

  if (!runner) {
    return (
      <div className="p-8">
        <div className="text-white">Runner not found</div>
      </div>
    )
  }

  const publicIPs = parsePublicIPs(runner.public_ips) || []
  const ipv4Addresses = filterIPv4(publicIPs) || []
  const gpus = parseGPUInfo(runner.gpu_info) || []

  return (
    <div className="p-4 flex flex-col min-h-0 h-full">
      <div className="mb-4 flex items-center gap-4 flex-wrap flex-shrink-0">
        <button
          onClick={() => navigate('/runners')}
          className="text-gray-400 hover:text-white text-sm"
        >
          ← Back
        </button>
        <h1 className="text-xl font-bold">{runner.name}</h1>
        <span className={`text-xs ${getStatusColor(runner.status)}`}>
          {runner.status === 'offline' ? 'Offline' : runner.status}
        </span>
        <div className="flex items-center gap-1.5">
          <MonitorIcon className="w-3 h-3" />
          <span className={`text-xs ${
            runner.screen_monitoring_enabled 
              ? 'text-green-400' 
              : 'text-gray-500'
          }`}>
            {runner.screen_monitoring_enabled 
              ? 'Monitoring' 
              : 'No monitoring'}
          </span>
        </div>
      </div>

      <div className="flex-1 grid grid-cols-1 lg:grid-cols-3 gap-6 min-h-0">
        {/* Resource Cards */}
        {showDeviceInfo && (
          <div className="lg:col-span-1 space-y-4">
            <GlassCard className="p-4">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold">Device Information</h2>
                <button
                  onClick={() => setShowDeviceInfo(false)}
                  className="text-gray-400 hover:text-white text-sm"
                  title="Hide device information"
                >
                  ✕
                </button>
              </div>
            <div className="space-y-3">
              {/* Hostname */}
              <div>
                <div className="text-gray-400 text-xs mb-1">Hostname</div>
                <div className="text-white">{runner.hostname}</div>
              </div>

              {/* IPv4 */}
              {ipv4Addresses.length > 0 && (
                <div className="flex items-center gap-2">
                  <IPIcon />
                  <div className="flex-1">
                    <div className="text-gray-400 text-xs mb-1">IPv4</div>
                    <div className="text-white font-mono text-sm">{ipv4Addresses[0]}</div>
                  </div>
                </div>
              )}

              {/* OS */}
              {(runner.os_version || runner.os) && (
                <div className="flex items-center gap-2">
                  <OSIcon />
                  <div className="flex-1">
                    <div className="text-gray-400 text-xs mb-1">OS</div>
                    <div className="text-white text-sm">{runner.os_version || runner.os}</div>
                  </div>
                </div>
              )}

              {/* CPU */}
              {(runner.cpu_model || runner.cpu_cores) && (
                <div className="flex items-center gap-2">
                  <CPUIcon />
                  <div className="flex-1">
                    <div className="text-gray-400 text-xs mb-1">CPU</div>
                    <div className="text-white text-sm">
                      {runner.cpu_model || `${runner.cpu_cores} cores`}
                      {runner.cpu_frequency_mhz && runner.cpu_frequency_mhz > 0 && (
                        <> @ {formatFrequency(runner.cpu_frequency_mhz)}</>
                      )}
                      {runner.cpu_cores && runner.cpu_cores > 0 && (
                        <> ({runner.cpu_cores} cores)</>
                      )}
                    </div>
                  </div>
                </div>
              )}

              {/* RAM */}
              {runner.memory_gb !== undefined && (
                <div className="flex items-center gap-2">
                  <RAMIcon />
                  <div className="flex-1">
                    <div className="text-gray-400 text-xs mb-1">RAM</div>
                    <div className="text-white text-sm">
                      {runner.memory_gb > 0 ? formatGB(runner.memory_gb) : 'Not detected'}
                    </div>
                  </div>
                </div>
              )}

              {/* Disk */}
              {(runner.disk_space_gb !== undefined || runner.total_disk_space_gb !== undefined) && (
                <div className="flex items-center gap-2">
                  <DiskIcon />
                  <div className="flex-1">
                    <div className="text-gray-400 text-xs mb-1">Disk</div>
                    <div className="text-white text-sm">
                      {runner.disk_space_gb !== undefined && runner.total_disk_space_gb !== undefined && runner.disk_space_gb > 0 && runner.total_disk_space_gb > 0 ? (
                        <>
                          {formatGB(runner.disk_space_gb)} / {formatGB(runner.total_disk_space_gb)} remaining
                        </>
                      ) : (
                        'Not detected'
                      )}
                    </div>
                  </div>
                </div>
              )}

              {/* GPU */}
              <div className="flex items-center gap-2">
                <GPUIcon />
                <div className="flex-1">
                  <div className="text-gray-400 text-xs mb-1">GPU</div>
                  <div className="text-white text-sm">
                    {gpus.length > 0 ? (
                      gpus.map((gpu, idx) => (
                        <div key={idx}>
                          {gpu.name}
                          {gpu.memory_gb > 0 && <> ({formatGB(gpu.memory_gb)} vRAM)</>}
                        </div>
                      ))
                    ) : (
                      'N/A'
                    )}
                  </div>
                </div>
              </div>
            </div>
          </GlassCard>
        </div>
        )}

        {/* Screen Stream Viewer */}
        <div className={`${showDeviceInfo ? "lg:col-span-2" : "lg:col-span-3"} flex flex-col min-h-0`}>
          {!showDeviceInfo && (
            <div className="mb-4 flex-shrink-0">
              <button
                onClick={() => setShowDeviceInfo(true)}
                className="text-sm text-gray-400 hover:text-white flex items-center gap-2"
                title="Show device information"
              >
                <span>ℹ️</span>
                <span>Show Device Information</span>
              </button>
            </div>
          )}
          <GlassCard className="p-4 flex flex-col flex-1 min-h-0">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold">Screen Monitoring</h2>
              {runner.screen_monitoring_enabled && (
                <div className="flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${
                    connectionStatus === 'connected' ? 'bg-green-400' : 
                    connectionStatus === 'connecting' ? 'bg-yellow-400' : 
                    'bg-gray-400'
                  }`} />
                  <span className={`text-sm ${getConnectionStatusColor()}`}>
                    {getConnectionStatusText()}
                  </span>
                </div>
              )}
            </div>

            {/* Quality and FPS Controls */}
            {runner.screen_monitoring_enabled && (
              <div className="mb-4 p-4 bg-gray-800/50 rounded-lg space-y-4">
                {/* Screen Selection Dropdown */}
                {availableScreens.length > 1 && (
                  <div>
                    <label className="text-sm text-gray-300 mb-2 block">Screen</label>
                    <select
                      value={selectedScreenIndex}
                      onChange={(e) => {
                        const newIndex = parseInt(e.target.value)
                        setSelectedScreenIndex(newIndex)
                        updateSettingsMutation.mutate({ quality, fps, screen_index: newIndex })
                      }}
                      className="w-full bg-gray-700 text-white text-sm rounded px-3 py-2 border border-gray-600 focus:border-blue-500 focus:outline-none"
                    >
                      {availableScreens.map((screen) => (
                        <option key={screen.index} value={screen.index}>
                          {screen.name || `Display ${screen.index + 1}`} ({screen.width}x{screen.height})
                          {screen.is_primary ? ' - Primary' : ''}
                        </option>
                      ))}
                    </select>
                  </div>
                )}

                <div>
                  <div className="flex items-center justify-between mb-2">
                    <label className="text-sm text-gray-300">Image Quality: {quality}</label>
                    <span className="text-xs text-gray-400">1-100</span>
                  </div>
                  <input
                    type="range"
                    min="1"
                    max="100"
                    value={quality}
                    onChange={(e) => {
                      const newQuality = parseInt(e.target.value)
                      setQuality(newQuality)
                    }}
                    onMouseUp={() => {
                      updateSettingsMutation.mutate({ quality, fps, screen_index: selectedScreenIndex })
                    }}
                    onTouchEnd={() => {
                      updateSettingsMutation.mutate({ quality, fps, screen_index: selectedScreenIndex })
                    }}
                    className="w-full h-2 bg-gray-700 rounded-lg appearance-none cursor-pointer accent-blue-500"
                  />
                  <div className="flex justify-between text-xs text-gray-500 mt-1">
                    <span>Low</span>
                    <span>High</span>
                  </div>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-2">
                    <label className="text-sm text-gray-300">Frame Rate: {fps.toFixed(1)} FPS</label>
                    <span className="text-xs text-gray-400">0.5-10</span>
                  </div>
                  <input
                    type="range"
                    min="0.5"
                    max="10"
                    step="0.5"
                    value={fps}
                    onChange={(e) => {
                      const newFps = parseFloat(e.target.value)
                      setFps(newFps)
                    }}
                    onMouseUp={() => {
                      updateSettingsMutation.mutate({ quality, fps, screen_index: selectedScreenIndex })
                    }}
                    onTouchEnd={() => {
                      updateSettingsMutation.mutate({ quality, fps, screen_index: selectedScreenIndex })
                    }}
                    className="w-full h-2 bg-gray-700 rounded-lg appearance-none cursor-pointer accent-blue-500"
                  />
                  <div className="flex justify-between text-xs text-gray-500 mt-1">
                    <span>0.5 FPS</span>
                    <span>10 FPS</span>
                  </div>
                </div>

                {updateSettingsMutation.isPending && (
                  <div className="text-xs text-blue-400">Saving settings...</div>
                )}
                {updateSettingsMutation.isSuccess && (
                  <div className="text-xs text-green-400">Settings saved</div>
                )}
                {updateSettingsMutation.isError && (
                  <div className="text-xs text-red-400">
                    Failed to save settings{axios.isAxiosError(updateSettingsMutation.error) && updateSettingsMutation.error.response?.data?.error
                      ? `: ${updateSettingsMutation.error.response.data.error}`
                      : ''}
                  </div>
                )}
              </div>
            )}
            
            {!runner.screen_monitoring_enabled ? (
              <div className="flex items-center justify-center flex-1 bg-gray-900 rounded-lg">
                <div className="text-center text-gray-400">
                  <MonitorIcon />
                  <p className="mt-4">Screen monitoring is not enabled for this device</p>
                  <p className="text-sm mt-2">Configure the solder agent to enable screen monitoring</p>
                </div>
              </div>
            ) : connectionStatus === 'connecting' ? (
              <div className="flex items-center justify-center flex-1 bg-gray-900 rounded-lg">
                <div className="text-center text-gray-400">
                  <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-white mx-auto mb-4"></div>
                  <p>Connecting to stream...</p>
                </div>
              </div>
            ) : connectionStatus === 'error' ? (
              <div className="flex items-center justify-center flex-1 bg-gray-900 rounded-lg">
                <div className="text-center text-gray-400">
                  <p className="text-red-400 mb-2">Connection Error</p>
                  <p className="text-sm">Attempting to reconnect...</p>
                </div>
              </div>
            ) : currentFrame ? (
              <div className="flex flex-col flex-1 min-h-0">
                {/* Live Stream Display */}
                <div className="relative bg-gray-900 rounded-lg mb-2 overflow-hidden flex-1 flex items-center justify-center min-h-0">
                  <img
                    src={currentFrame}
                    alt="Live screen stream"
                    className="max-w-full max-h-full"
                    style={{ objectFit: 'contain' }}
                  />
                </div>
                <div className="text-xs text-gray-400 text-center flex-shrink-0">
                  Live stream - frames update automatically
                </div>
              </div>
            ) : (
              <div className="flex items-center justify-center flex-1 bg-gray-900 rounded-lg">
                <div className="text-center text-gray-400">
                  <p>Waiting for stream...</p>
                  <p className="text-sm mt-2">Frames will appear here when the agent starts streaming</p>
                </div>
              </div>
            )}
          </GlassCard>
        </div>
      </div>
    </div>
  )
}

