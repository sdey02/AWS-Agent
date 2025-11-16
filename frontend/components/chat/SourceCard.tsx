'use client';

interface Source {
  type: string;
  url: string;
  confidence: number;
}

export default function SourceCard({ source }: { source: Source }) {
  return (
    <a
      href={source.url}
      target="_blank"
      rel="noopener noreferrer"
      className="block p-3 bg-white border border-gray-200 rounded-lg hover:border-blue-500 hover:shadow-sm transition-all"
    >
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <div className="flex items-center space-x-2">
            <span className="text-xs font-medium text-blue-600 uppercase">
              {source.type === 'kg' ? 'Knowledge Graph' : 'Documentation'}
            </span>
            <span className="text-xs text-gray-500">
              {(source.confidence * 100).toFixed(0)}% confidence
            </span>
          </div>
          <p className="text-sm text-gray-700 mt-1 truncate">{source.url}</p>
        </div>
        <svg
          className="w-4 h-4 text-gray-400 flex-shrink-0 ml-2"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
          />
        </svg>
      </div>
    </a>
  );
}
