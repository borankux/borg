import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import Layout from './components/Layout'
import ProtectedRoute from './components/ProtectedRoute'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Jobs from './pages/Jobs'
import Runners from './pages/Runners'
import Logs from './pages/Logs'
import Download from './pages/Download'
import DeviceDetail from './pages/DeviceDetail'

const queryClient = new QueryClient()

function AppRoutes() {
  const { isAuthenticated } = useAuth()

  return (
    <Routes>
      <Route path="/login" element={isAuthenticated ? <Navigate to="/" replace /> : <Login />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <Layout>
              <Dashboard />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/jobs"
        element={
          <ProtectedRoute>
            <Layout>
              <Jobs />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/runners"
        element={
          <ProtectedRoute>
            <Layout>
              <Runners />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/runners/:id/monitor"
        element={
          <ProtectedRoute>
            <Layout>
              <DeviceDetail />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/logs/:taskId?"
        element={
          <ProtectedRoute>
            <Layout>
              <Logs />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route
        path="/download"
        element={
          <ProtectedRoute>
            <Layout>
              <Download />
            </Layout>
          </ProtectedRoute>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </AuthProvider>
    </QueryClientProvider>
  )
}

export default App
