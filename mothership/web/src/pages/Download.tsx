import { useState } from 'react'
import GlassCard from '../components/GlassCard'
import Modal from '../components/Modal'

export default function Download() {
  const [downloading, setDownloading] = useState(false)
  const [errorModal, setErrorModal] = useState<{ isOpen: boolean; message: string }>({
    isOpen: false,
    message: '',
  })

  const handleDownload = async (platform: 'windows' | 'linux' | 'macos') => {
    setDownloading(true)
    try {
      const response = await fetch('/api/v1/download/solder.exe')
      const contentType = response.headers.get('content-type')
      
      if (response.ok && contentType && contentType.startsWith('application/')) {
        // Binary file response
        const blob = await response.blob()
        const url = window.URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        const extension = platform === 'windows' ? 'exe' : platform === 'macos' ? 'macos' : 'linux'
        a.download = `solder-${extension}.${platform === 'windows' ? 'exe' : ''}`
        document.body.appendChild(a)
        a.click()
        window.URL.revokeObjectURL(url)
        document.body.removeChild(a)
      } else {
        // JSON response (not implemented message)
        const data = await response.json()
        setErrorModal({
          isOpen: true,
          message: data.message || 'Download not available. Please build from source using the instructions below.',
        })
      }
    } catch (error) {
      console.error('Download failed:', error)
      setErrorModal({
        isOpen: true,
        message: 'Download failed. Please build from source using the instructions below.',
      })
    } finally {
      setDownloading(false)
    }
  }

  return (
    <div className="p-8">
      <h1 className="text-3xl font-bold mb-8">Download Solder</h1>
      
      <div className="space-y-6">
        {/* Download Options */}
        <GlassCard>
          <h2 className="text-2xl font-semibold mb-6">Download Solder Agent</h2>
          <p className="text-gray-400 mb-6">
            Solder is the worker agent that connects to the distributed task execution system and executes tasks.
          </p>
          
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
            <button
              onClick={() => handleDownload('windows')}
              disabled={downloading}
              className="flex flex-col items-center justify-center p-6 bg-gray-800 hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
            >
              <div className="text-4xl mb-2">🪟</div>
              <div className="font-semibold">Windows</div>
              <div className="text-sm text-gray-400 mt-1">.exe</div>
            </button>
            
            <button
              onClick={() => handleDownload('linux')}
              disabled={downloading}
              className="flex flex-col items-center justify-center p-6 bg-gray-800 hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
            >
              <div className="text-4xl mb-2">🐧</div>
              <div className="font-semibold">Linux</div>
              <div className="text-sm text-gray-400 mt-1">Binary</div>
            </button>
            
            <button
              onClick={() => handleDownload('macos')}
              disabled={downloading}
              className="flex flex-col items-center justify-center p-6 bg-gray-800 hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
            >
              <div className="text-4xl mb-2">🍎</div>
              <div className="font-semibold">macOS</div>
              <div className="text-sm text-gray-400 mt-1">Binary</div>
            </button>
          </div>
        </GlassCard>

        {/* Usage Instructions */}
        <GlassCard>
          <h2 className="text-2xl font-semibold mb-4">Usage Instructions</h2>
          
          <div className="space-y-6 text-gray-300">
            <section>
              <h3 className="text-xl font-semibold text-white mb-3">1. Download and Extract</h3>
              <p className="mb-2">
                Download the appropriate binary for your operating system and extract it to a directory of your choice.
              </p>
            </section>

            <section>
              <h3 className="text-xl font-semibold text-white mb-3">2. Set Environment Variables</h3>
              <p className="mb-3">Configure the following environment variables:</p>
              <div className="bg-gray-800 p-4 rounded font-mono text-sm space-y-2">
                <div>
                  <span className="text-blue-400">MOTHERSHIP_ADDR</span>
                  <span className="text-gray-500">=</span>
                  <span className="text-green-400">http://localhost:8080</span>
                  <span className="text-gray-500 ml-2"># Server address</span>
                </div>
                <div>
                  <span className="text-blue-400">RUNNER_NAME</span>
                  <span className="text-gray-500">=</span>
                  <span className="text-green-400">my-runner</span>
                  <span className="text-gray-500 ml-2"># Optional: custom name</span>
                </div>
                <div>
                  <span className="text-blue-400">RUNNER_TOKEN</span>
                  <span className="text-gray-500">=</span>
                  <span className="text-green-400">default-token</span>
                  <span className="text-gray-500 ml-2"># Authentication token</span>
                </div>
                <div>
                  <span className="text-blue-400">WORK_DIR</span>
                  <span className="text-gray-500">=</span>
                  <span className="text-green-400">./work</span>
                  <span className="text-gray-500 ml-2"># Working directory for tasks</span>
                </div>
              </div>
            </section>

            <section>
              <h3 className="text-xl font-semibold text-white mb-3">3. Run the Agent</h3>
              
              <div className="space-y-3">
                <div>
                  <p className="mb-2 font-semibold">Windows (PowerShell):</p>
                  <div className="bg-gray-800 p-4 rounded font-mono text-sm">
                    <div className="text-gray-500">$env:MOTHERSHIP_ADDR="http://localhost:8080"</div>
                    <div className="text-gray-500">$env:RUNNER_NAME="my-runner"</div>
                    <div className="text-gray-500">$env:RUNNER_TOKEN="default-token"</div>
                    <div className="text-gray-500">$env:WORK_DIR=".\work"</div>
                    <div className="text-white mt-2">.\solder.exe</div>
                  </div>
                </div>

                <div>
                  <p className="mb-2 font-semibold">Linux/macOS:</p>
                  <div className="bg-gray-800 p-4 rounded font-mono text-sm">
                    <div className="text-gray-500">export MOTHERSHIP_ADDR="http://localhost:8080"</div>
                    <div className="text-gray-500">export RUNNER_NAME="my-runner"</div>
                    <div className="text-gray-500">export RUNNER_TOKEN="default-token"</div>
                    <div className="text-gray-500">export WORK_DIR="./work"</div>
                    <div className="text-white mt-2">chmod +x solder</div>
                    <div className="text-white">./solder</div>
                  </div>
                </div>
              </div>
            </section>

            <section>
              <h3 className="text-xl font-semibold text-white mb-3">4. Verify Connection</h3>
              <p>
                Once running, the agent will automatically register with the distributed task execution system.
                You can verify the connection by checking the Runners page in the web dashboard.
              </p>
            </section>

            <section>
              <h3 className="text-xl font-semibold text-white mb-3">Features</h3>
              <ul className="list-disc list-inside space-y-2 ml-4">
                <li>Automatic resource detection (CPU, RAM, Disk, GPU, Public IP)</li>
                <li>Heartbeat monitoring for health checks</li>
                <li>Support for shell scripts, binaries, and Docker containers</li>
                <li>Automatic file download and artifact upload</li>
                <li>Task execution with real-time log streaming</li>
              </ul>
            </section>

            <section>
              <h3 className="text-xl font-semibold text-white mb-3">Troubleshooting</h3>
              <div className="space-y-2">
                <p><strong>Connection Issues:</strong> Ensure the MOTHERSHIP_ADDR is correct and the server is running.</p>
                <p><strong>Registration Failed:</strong> Check that RUNNER_TOKEN matches the server configuration.</p>
                <p><strong>Tasks Not Executing:</strong> Verify the WORK_DIR exists and has write permissions.</p>
              </div>
            </section>
          </div>
        </GlassCard>
      </div>

      <Modal
        isOpen={errorModal.isOpen}
        onClose={() => setErrorModal({ isOpen: false, message: '' })}
        title="Download Information"
        showCancel={false}
        confirmText="OK"
        onConfirm={() => setErrorModal({ isOpen: false, message: '' })}
      >
        <p className="text-gray-300">{errorModal.message}</p>
      </Modal>
    </div>
  )
}

