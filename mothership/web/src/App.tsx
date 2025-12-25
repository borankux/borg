import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Jobs from './pages/Jobs'
import Runners from './pages/Runners'
import Logs from './pages/Logs'
import Download from './pages/Download'
import DeviceDetail from './pages/DeviceDetail'

const queryClient = new QueryClient()

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/jobs" element={<Jobs />} />
            <Route path="/runners" element={<Runners />} />
            <Route path="/runners/:id/monitor" element={<DeviceDetail />} />
            <Route path="/logs/:taskId?" element={<Logs />} />
            <Route path="/download" element={<Download />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
