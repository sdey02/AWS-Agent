'use client';

import { useState } from 'react';
import ChatInterface from '@/components/chat/ChatInterface';

export default function Home() {
  return (
    <main className="flex min-h-screen flex-col">
      <header className="bg-blue-600 text-white p-4">
        <div className="container mx-auto">
          <h1 className="text-2xl font-bold">AWS RAG Agent</h1>
          <p className="text-sm text-blue-100">AI-Powered AWS Issue Resolution Assistant</p>
        </div>
      </header>

      <div className="flex-1 container mx-auto p-4">
        <ChatInterface />
      </div>
    </main>
  );
}
