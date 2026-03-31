import { ChatPanel, ChatAPIProvider, createChatAPI } from 'chat-component';
import 'chat-component/styles.css';

// Create a ChatAPI client pointing at the same origin (chat-service serves both UI and API).
const api = createChatAPI(window.location.origin);

export function App() {
  return (
    <ChatAPIProvider value={api}>
      <ChatPanel
        opened
        onClose={() => {}}
        fullPage
      />
    </ChatAPIProvider>
  );
}
