'use client';

import { useState } from 'react';
import MessageBubble from './MessageBubble';
import SourceCard from './SourceCard';
import FeedbackButton from './FeedbackButton';

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  sources?: Source[];
  confidence?: number;
}

interface Source {
  type: string;
  url: string;
  chunk_id?: string;
  confidence: number;
}

export default function ChatInterface() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || isLoading) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      role: 'user',
      content: input,
    };

    setMessages(prev => [...prev, userMessage]);
    setInput('');
    setIsLoading(true);

    try {
      const response = await fetch('http://localhost:8080/api/v1/query', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          query: input,
          user_id: 'default_user',
        }),
      });

      const data = await response.json();

      const assistantMessage: Message = {
        id: data.id,
        role: 'assistant',
        content: data.response,
        sources: data.sources,
        confidence: data.confidence,
      };

      setMessages(prev => [...prev, assistantMessage]);
    } catch (error) {
      console.error('Error sending message:', error);
      const errorMessage: Message = {
        id: Date.now().toString(),
        role: 'assistant',
        content: 'Sorry, I encountered an error processing your request. Please try again.',
      };
      setMessages(prev => [...prev, errorMessage]);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex flex-col h-[calc(100vh-120px)] bg-white rounded-lg shadow-lg">
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="text-center text-gray-500 mt-8">
            <h2 className="text-xl font-semibold mb-2">Welcome to AWS RAG Agent</h2>
            <p>Ask me about AWS issues and I'll help you resolve them!</p>
            <div className="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4 max-w-2xl mx-auto">
              <button
                onClick={() => setInput('My Lambda function is timing out when accessing S3')}
                className="p-4 text-left border rounded-lg hover:bg-gray-50"
              >
                <p className="font-medium">Lambda timeout issue</p>
                <p className="text-sm text-gray-600">Help with Lambda timing out when accessing S3</p>
              </button>
              <button
                onClick={() => setInput('EC2 instance won\'t start')}
                className="p-4 text-left border rounded-lg hover:bg-gray-50"
              >
                <p className="font-medium">EC2 startup problem</p>
                <p className="text-sm text-gray-600">Troubleshoot EC2 instance startup issues</p>
              </button>
            </div>
          </div>
        )}

        {messages.map((msg) => (
          <div key={msg.id} className="space-y-2">
            <MessageBubble message={msg} />

            {msg.role === 'assistant' && msg.sources && msg.sources.length > 0 && (
              <div className="ml-12">
                <p className="text-sm text-gray-600 mb-2">Sources:</p>
                <div className="space-y-2">
                  {msg.sources.slice(0, 5).map((source, idx) => (
                    <SourceCard key={idx} source={source} />
                  ))}
                </div>
              </div>
            )}

            {msg.role === 'assistant' && (
              <div className="ml-12">
                <FeedbackButton messageId={msg.id} />
              </div>
            )}
          </div>
        ))}

        {isLoading && (
          <div className="flex items-center space-x-2 text-gray-500">
            <div className="animate-pulse">Thinking...</div>
          </div>
        )}
      </div>

      <form onSubmit={handleSubmit} className="border-t p-4 bg-gray-50">
        <div className="flex gap-2">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleSubmit(e);
              }
            }}
            placeholder="Describe your AWS issue..."
            className="flex-1 p-3 border rounded-lg resize-none focus:outline-none focus:ring-2 focus:ring-blue-500"
            rows={3}
            disabled={isLoading}
          />
          <button
            type="submit"
            disabled={isLoading || !input.trim()}
            className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
          >
            {isLoading ? 'Sending...' : 'Send'}
          </button>
        </div>
      </form>
    </div>
  );
}
