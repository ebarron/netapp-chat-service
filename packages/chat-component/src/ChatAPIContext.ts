import { createContext, useContext } from 'react';
import type { ChatAPI } from './ChatAPI';

const ChatAPIContext = createContext<ChatAPI | null>(null);

export const ChatAPIProvider = ChatAPIContext.Provider;

export function useChatAPI(): ChatAPI {
  const api = useContext(ChatAPIContext);
  if (!api) {
    throw new Error('useChatAPI must be used within a ChatAPIProvider');
  }
  return api;
}
