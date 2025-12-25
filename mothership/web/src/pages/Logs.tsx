import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import axios from 'axios'
import GlassCard from '../components/GlassCard'

interface LogEntry {
  id: number
  task_id: string
  level: string
  message: string
  timestamp: string
}

export default function Logs() {
  const { taskId } = useParams()
  const logContainerRef = useRef<HTMLDivElement>(null)
  
  const { data: logs } = useQuery<LogEntry[]>({
    queryKey: ['logs', taskId],
    queryFn: async () => {
      if (!taskId) return []
      const res = await axios.get(`/api/v1/tasks/${taskId}/logs`)
      return res.data
    },
    enabled: !!taskId,
    refetchInterval: 2000,
  })
  
  useEffect(() => {
    if (logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
  }, [logs])
  
  const getLogColor = (level: string) => {
    switch (level) {
      case 'error':
      case 'stderr':
        return 'text-red-400'
      case 'stdout':
        return 'text-gray-300'
      default:
        return 'text-gray-400'
    }
  }
  
  return (
    <div className="p-8">
      <h1 className="text-3xl font-bold mb-8">Logs</h1>
      
      <GlassCard>
        {taskId ? (
          <div
            ref={logContainerRef}
            className="h-[600px] overflow-y-auto font-mono text-sm space-y-1"
          >
            {logs?.map((log) => (
              <div key={log.id} className={getLogColor(log.level)}>
                <span className="text-gray-500">
                  [{new Date(log.timestamp).toLocaleTimeString()}]
                </span>{' '}
                <span className="text-gray-600">[{log.level}]</span>{' '}
                {log.message}
              </div>
            ))}
            {!logs || logs.length === 0 && (
              <div className="text-gray-400">No logs available</div>
            )}
          </div>
        ) : (
          <div className="text-center text-gray-400 py-8">
            Select a task to view logs
          </div>
        )}
      </GlassCard>
    </div>
  )
}

