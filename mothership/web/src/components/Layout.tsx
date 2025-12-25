import { Link, useLocation } from 'react-router-dom'
import { DashboardIcon, JobsIcon, RunnersIcon, LogsIcon, DownloadIcon } from './Icons'

interface LayoutProps {
  children: React.ReactNode
}

export default function Layout({ children }: LayoutProps) {
  const location = useLocation()
  
  const navItems = [
    { path: '/', label: 'Dashboard', icon: DashboardIcon },
    { path: '/jobs', label: 'Jobs', icon: JobsIcon },
    { path: '/runners', label: 'Runners', icon: RunnersIcon },
    { path: '/logs', label: 'Logs', icon: LogsIcon },
    { path: '/download', label: 'Download', icon: DownloadIcon },
  ]
  
  return (
    <div className="flex h-screen bg-black text-white">
      {/* Sidebar */}
      <div className="w-64 bg-gray-900 border-r border-gray-800 flex flex-col">
        <div className="p-6 border-b border-gray-800">
          <h1 className="text-2xl font-bold">BORG</h1>
          <p className="text-sm text-gray-400 mt-1">Distributed Task Execution System</p>
        </div>
        <nav className="flex-1 p-4 space-y-2">
          {navItems.map((item) => {
            const isActive = location.pathname === item.path
            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center space-x-3 px-4 py-3 rounded-lg transition-colors ${
                  isActive
                    ? 'bg-white text-black'
                    : 'text-gray-300 hover:bg-gray-800 hover:text-white'
                }`}
              >
                <item.icon />
                <span className="font-medium">{item.label}</span>
              </Link>
            )
          })}
        </nav>
      </div>
      
      {/* Main content */}
      <div className="flex-1 overflow-auto">
        {children}
      </div>
    </div>
  )
}

