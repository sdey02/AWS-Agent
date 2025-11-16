import type { Metadata } from 'next'
import './globals.css'

export const metadata: Metadata = {
  title: 'AWS RAG Agent',
  description: 'AI-powered AWS issue resolution assistant',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en">
      <body className="bg-gray-50">{children}</body>
    </html>
  )
}
