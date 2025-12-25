import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
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
}

interface Screenshot {
  filename: string
  timestamp: number
  size: number
}

export default function DeviceDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [currentIndex, setCurrentIndex] = useState(0)
  const [imageError, setImageError] = useState(false)

  const { data: runner, isLoading: runnerLoading } = useQuery<Runner>({
    queryKey: ['runner', id],
    queryFn: async () => {
      const res = await axios.get(`/api/v1/runners/${id}`)
      return res.data
    },
    enabled: !!id,
    refetchInterval: 5000,
  })

  const isScreenMonitoringEnabled = runner?.screen_monitoring_enabled === true

  const { data: screenshotsData, isLoading: screenshotsLoading } = useQuery<{ screenshots: Screenshot[] }>({
    queryKey: ['screenshots', id],
    queryFn: async () => {
      const res = await axios.get(`/api/v1/runners/${id}/screenshots?limit=50`)
      return res.data
    },
    enabled: !!id && isScreenMonitoringEnabled,
    refetchInterval: isScreenMonitoringEnabled ? 5000 : false,
  })

  const screenshots = screenshotsData?.screenshots || []

  useEffect(() => {
    if (screenshots.length > 0 && currentIndex >= screenshots.length) {
      setCurrentIndex(screenshots.length - 1)
    }
  }, [screenshots.length, currentIndex])

  useEffect(() => {
    setImageError(false)
  }, [currentIndex])

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

  const currentScreenshot = screenshots[currentIndex]

  const handlePrev = () => {
    if (currentIndex > 0) {
      setCurrentIndex(currentIndex - 1)
      setImageError(false)
    }
  }

  const handleNext = () => {
    if (currentIndex < screenshots.length - 1) {
      setCurrentIndex(currentIndex + 1)
      setImageError(false)
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
    <div className="p-8">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <button
            onClick={() => navigate('/runners')}
            className="text-gray-400 hover:text-white mb-2"
          >
            ← Back to Runners
          </button>
          <h1 className="text-3xl font-bold">{runner.name}</h1>
          <div className="flex items-center gap-4 mt-2">
            <span className={`text-sm ${getStatusColor(runner.status)}`}>
              {runner.status === 'offline' ? 'Offline' : runner.status}
            </span>
            <div className="flex items-center gap-2">
              <MonitorIcon />
              <span className={`text-sm ${
                runner.screen_monitoring_enabled 
                  ? 'text-green-400' 
                  : 'text-gray-500'
              }`}>
                {runner.screen_monitoring_enabled 
                  ? 'Screen monitoring enabled' 
                  : 'Screen monitoring disabled'}
              </span>
            </div>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Resource Cards */}
        <div className="lg:col-span-1 space-y-4">
          <GlassCard className="p-4">
            <h2 className="text-lg font-semibold mb-4">Device Information</h2>
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

        {/* Screenshot Viewer */}
        <div className="lg:col-span-2">
          <GlassCard className="p-4">
            <h2 className="text-lg font-semibold mb-4">Screen Monitoring</h2>
            
            {!runner.screen_monitoring_enabled ? (
              <div className="flex items-center justify-center h-96 bg-gray-900 rounded-lg">
                <div className="text-center text-gray-400">
                  <MonitorIcon />
                  <p className="mt-4">Screen monitoring is not enabled for this device</p>
                  <p className="text-sm mt-2">Configure the solder agent to enable screen monitoring</p>
                </div>
              </div>
            ) : screenshotsLoading ? (
              <div className="flex items-center justify-center h-96">
                <div className="text-white">Loading screenshots...</div>
              </div>
            ) : screenshots.length === 0 ? (
              <div className="flex items-center justify-center h-96 bg-gray-900 rounded-lg">
                <div className="text-center text-gray-400">
                  <p>No screenshots available yet</p>
                  <p className="text-sm mt-2">Screenshots will appear here when captured</p>
                </div>
              </div>
            ) : (
              <div>
                {/* Image Display */}
                <div className="relative bg-gray-900 rounded-lg mb-4 overflow-hidden" style={{ minHeight: '400px' }}>
                  {currentScreenshot && (
                    <img
                      src={`/api/v1/runners/${id}/screenshots/${currentScreenshot.filename}`}
                      alt={`Screenshot ${currentIndex + 1}`}
                      className={`w-full h-auto ${imageError ? 'hidden' : ''}`}
                      onError={() => setImageError(true)}
                      style={{ maxHeight: '600px', objectFit: 'contain' }}
                    />
                  )}
                  {imageError && (
                    <div className="flex items-center justify-center h-96 text-gray-400">
                      <p>Failed to load image</p>
                    </div>
                  )}
                </div>

                {/* Navigation Controls */}
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-4">
                    <button
                      onClick={handlePrev}
                      disabled={currentIndex === 0}
                      className={`px-4 py-2 rounded ${
                        currentIndex === 0
                          ? 'bg-gray-700 text-gray-500 cursor-not-allowed'
                          : 'bg-blue-600 text-white hover:bg-blue-700'
                      }`}
                    >
                      Previous
                    </button>
                    <span className="text-gray-400">
                      {currentIndex + 1} / {screenshots.length}
                    </span>
                    <button
                      onClick={handleNext}
                      disabled={currentIndex >= screenshots.length - 1}
                      className={`px-4 py-2 rounded ${
                        currentIndex >= screenshots.length - 1
                          ? 'bg-gray-700 text-gray-500 cursor-not-allowed'
                          : 'bg-blue-600 text-white hover:bg-blue-700'
                      }`}
                    >
                      Next
                    </button>
                  </div>
                  {currentScreenshot && (
                    <div className="text-xs text-gray-400">
                      {new Date(currentScreenshot.timestamp * 1000).toLocaleString()}
                    </div>
                  )}
                </div>

                {/* Thumbnail Strip */}
                <div className="flex gap-2 overflow-x-auto pb-2">
                  {screenshots.slice(0, 20).map((screenshot, idx) => (
                    <button
                      key={idx}
                      onClick={() => {
                        setCurrentIndex(idx)
                        setImageError(false)
                      }}
                      className={`flex-shrink-0 w-20 h-12 bg-gray-800 rounded border-2 ${
                        idx === currentIndex ? 'border-blue-500' : 'border-gray-700'
                      } overflow-hidden`}
                    >
                      <img
                        src={`/api/v1/runners/${id}/screenshots/${screenshot.filename}`}
                        alt={`Thumbnail ${idx + 1}`}
                        className="w-full h-full object-cover"
                      />
                    </button>
                  ))}
                </div>
              </div>
            )}
          </GlassCard>
        </div>
      </div>
    </div>
  )
}

