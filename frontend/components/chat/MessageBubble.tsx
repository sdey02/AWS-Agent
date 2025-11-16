'use client';

import ReactMarkdown from 'react-markdown';

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  confidence?: number;
}

export default function MessageBubble({ message }: { message: Message }) {
  const isUser = message.role === 'user';

  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-3xl rounded-lg p-4 ${
          isUser
            ? 'bg-blue-600 text-white'
            : 'bg-gray-100 text-gray-900'
        }`}
      >
        {isUser ? (
          <p>{message.content}</p>
        ) : (
          <div className="prose prose-sm max-w-none">
            <ReactMarkdown>{message.content}</ReactMarkdown>
          </div>
        )}

        {!isUser && message.confidence !== undefined && (
          <div className="mt-2 pt-2 border-t border-gray-300">
            <p className="text-xs text-gray-600">
              Confidence: {(message.confidence * 100).toFixed(0)}%
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
