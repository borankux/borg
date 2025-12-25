import React from 'react'

interface BentoGridProps {
  children: React.ReactNode
  className?: string
}

export default function BentoGrid({ children, className = '' }: BentoGridProps) {
  return (
    <div className={`grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 ${className}`}>
      {children}
    </div>
  )
}

