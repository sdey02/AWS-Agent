'use client';

import { useState } from 'react';

export default function FeedbackButton({ messageId }: { messageId: string }) {
  const [feedback, setFeedback] = useState<'helpful' | 'not-helpful' | null>(null);

  const handleFeedback = async (helpful: boolean) => {
    setFeedback(helpful ? 'helpful' : 'not-helpful');

    try {
      await fetch('http://localhost:8080/api/v1/feedback', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          query_id: messageId,
          helpful,
        }),
      });
    } catch (error) {
      console.error('Error submitting feedback:', error);
    }
  };

  if (feedback) {
    return (
      <p className="text-sm text-gray-500">
        Thank you for your feedback!
      </p>
    );
  }

  return (
    <div className="flex items-center space-x-2">
      <span className="text-sm text-gray-600">Was this helpful?</span>
      <button
        onClick={() => handleFeedback(true)}
        className="p-1 hover:bg-gray-100 rounded"
        title="Helpful"
      >
        👍
      </button>
      <button
        onClick={() => handleFeedback(false)}
        className="p-1 hover:bg-gray-100 rounded"
        title="Not helpful"
      >
        👎
      </button>
    </div>
  );
}
