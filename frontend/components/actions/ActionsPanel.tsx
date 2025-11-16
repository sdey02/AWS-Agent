'use client';

import { useState } from 'react';

interface Action {
  service: string;
  action: string;
  parameters: Record<string, any>;
  description: string;
  risk_level: string;
}

interface ActionPlan {
  actions: Action[];
  explanation: string;
  risk_level: string;
  requires_approval: boolean;
}

export default function ActionsPanel({ issue, context }: { issue: string; context: string }) {
  const [plan, setPlan] = useState<ActionPlan | null>(null);
  const [isPlanning, setIsPlanning] = useState(false);
  const [isExecuting, setIsExecuting] = useState(false);
  const [results, setResults] = useState<any[]>([]);

  const handlePlanActions = async () => {
    setIsPlanning(true);

    try {
      const response = await fetch('http://localhost:8080/api/v1/actions/plan', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ issue, context }),
      });

      const data = await response.json();
      setPlan({
        actions: data.plan || [],
        explanation: data.explanation || '',
        risk_level: data.risk_level || 'MEDIUM',
        requires_approval: data.requires_approval || false,
      });
    } catch (error) {
      console.error('Failed to plan actions:', error);
    } finally {
      setIsPlanning(false);
    }
  };

  const handleExecuteActions = async (approved: boolean) => {
    if (!plan) return;

    setIsExecuting(true);

    try {
      const response = await fetch('http://localhost:8080/api/v1/actions/execute', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ plan, approved }),
      });

      const data = await response.json();
      setResults(data.results || []);
    } catch (error) {
      console.error('Failed to execute actions:', error);
    } finally {
      setIsExecuting(false);
    }
  };

  const getRiskColor = (risk: string) => {
    switch (risk) {
      case 'LOW':
        return 'text-green-600 bg-green-100';
      case 'MEDIUM':
        return 'text-yellow-600 bg-yellow-100';
      case 'HIGH':
        return 'text-red-600 bg-red-100';
      default:
        return 'text-gray-600 bg-gray-100';
    }
  };

  return (
    <div className="border rounded-lg p-4 bg-white">
      <h3 className="text-lg font-semibold mb-4">AWS Actions</h3>

      {!plan && !isPlanning && (
        <button
          onClick={handlePlanActions}
          className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          Plan Automated Fix
        </button>
      )}

      {isPlanning && (
        <div className="text-gray-600">Planning actions...</div>
      )}

      {plan && !results.length && (
        <div className="space-y-4">
          <div className="flex items-center space-x-2">
            <span className="text-sm font-medium">Risk Level:</span>
            <span className={`px-2 py-1 rounded text-xs font-medium ${getRiskColor(plan.risk_level)}`}>
              {plan.risk_level}
            </span>
          </div>

          <div className="bg-gray-50 p-3 rounded">
            <p className="text-sm text-gray-700">{plan.explanation}</p>
          </div>

          <div className="space-y-2">
            <p className="text-sm font-medium">Planned Actions:</p>
            {plan.actions.map((action, idx) => (
              <div key={idx} className="border-l-4 border-blue-500 pl-3 py-2 bg-gray-50">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <p className="font-medium text-sm">
                      {action.service}.{action.action}
                    </p>
                    <p className="text-sm text-gray-600 mt-1">{action.description}</p>
                    <div className="mt-2">
                      <span className="text-xs text-gray-500">Parameters:</span>
                      <pre className="text-xs bg-white p-2 rounded mt-1">
                        {JSON.stringify(action.parameters, null, 2)}
                      </pre>
                    </div>
                  </div>
                  <span className={`ml-2 px-2 py-1 rounded text-xs ${getRiskColor(action.risk_level)}`}>
                    {action.risk_level}
                  </span>
                </div>
              </div>
            ))}
          </div>

          <div className="flex space-x-2">
            <button
              onClick={() => handleExecuteActions(true)}
              disabled={isExecuting}
              className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700 disabled:bg-gray-300"
            >
              {isExecuting ? 'Executing...' : plan.requires_approval ? 'Approve & Execute' : 'Execute'}
            </button>
            <button
              onClick={() => setPlan(null)}
              className="px-4 py-2 border border-gray-300 rounded hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>

          {plan.requires_approval && (
            <p className="text-sm text-yellow-600 flex items-start">
              <svg className="w-4 h-4 mr-1 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
              </svg>
              This action requires your approval before execution
            </p>
          )}
        </div>
      )}

      {results.length > 0 && (
        <div className="space-y-2">
          <p className="text-sm font-medium">Execution Results:</p>
          {results.map((result, idx) => (
            <div
              key={idx}
              className={`border-l-4 pl-3 py-2 ${
                result.success ? 'border-green-500 bg-green-50' : 'border-red-500 bg-red-50'
              }`}
            >
              <div className="flex items-start justify-between">
                <div>
                  <p className="font-medium text-sm">
                    {result.action.service}.{result.action.action}
                  </p>
                  <p className="text-sm text-gray-700 mt-1">
                    {result.success ? result.output : result.error}
                  </p>
                </div>
                <span className="text-xs">
                  {result.success ? '✓ Success' : '✗ Failed'}
                </span>
              </div>
            </div>
          ))}

          <button
            onClick={() => {
              setPlan(null);
              setResults([]);
            }}
            className="mt-4 px-4 py-2 border border-gray-300 rounded hover:bg-gray-50"
          >
            Done
          </button>
        </div>
      )}
    </div>
  );
}
