import React from 'react'

interface GlassCardProps {
  children: React.ReactNode
  className?: string
  colSpan?: number
  rowSpan?: number
}

export default function GlassCard({ children, className = '', colSpan = 1, rowSpan = 1 }: GlassCardProps) {
  return (
    <div
      className={`bg-white/5 backdrop-blur-md border border-white/10 rounded-xl p-6 ${className}`}
      style={{
        gridColumn: `span ${colSpan}`,
        gridRow: `span ${rowSpan}`,
      }}
    >
      {children}
    </div>
  )
}

