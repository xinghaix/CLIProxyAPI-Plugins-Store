import { describe, expect, it } from 'vitest';
import { requestProtocol, requestProtocolLabel } from './requestProtocol.js';

describe('requestProtocol', () => {
  it('maps websocket executors to websocket', () => {
    expect(requestProtocol({ executor_type: 'CodexWebsocketsExecutor' })).toBe('websocket');
    expect(requestProtocol({ executorType: 'XAIWebsocketsExecutor' })).toBe('websocket');
  });

  it('defaults ordinary executors and empty rows to http', () => {
    expect(requestProtocol({ executor_type: 'OpenAICompatExecutor' })).toBe('http');
    expect(requestProtocol({})).toBe('http');
  });

  it('prefers an explicit protocol field from the API', () => {
    expect(requestProtocol({ protocol: 'WebSocket', executor_type: 'OpenAICompatExecutor' })).toBe('websocket');
    expect(requestProtocol({ protocol: 'HTTP' })).toBe('http');
  });
});

describe('requestProtocolLabel', () => {
  it('renders HTTP and WebSocket for the status column', () => {
    expect(requestProtocolLabel({ executor_type: 'OpenAICompatExecutor' })).toBe('HTTP');
    expect(requestProtocolLabel({ executor_type: 'CodexWebsocketsExecutor' })).toBe('WebSocket');
  });
});
