// WebSocket connection and chat state
let ws = null;
let currentSessionId = null;
let isConnected = false;
let reconnectAttempts = 0;
let maxReconnectAttempts = 10;
let reconnectDelay = 1000;
let messageBuffer = '';
let isProcessing = false;
let connectionStatus = 'disconnected';

// DOM elements
const chatMessages = document.getElementById('chatMessages');
const chatInput = document.getElementById('chatInput');
const chatForm = document.getElementById('chatForm');
const sendButton = document.getElementById('sendButton');
const newChatBtn = document.getElementById('newChatBtn');
const sessionsList = document.getElementById('sessionsList');
const menuToggle = document.getElementById('menuToggle');
const sidebar = document.querySelector('.sidebar');
const connectionStatusEl = document.getElementById('connectionStatus');

// Initialize marked for markdown rendering
marked.setOptions({
    highlight: function (code, lang) {
        if (lang && hljs.getLanguage(lang)) {
            return hljs.highlight(code, { language: lang }).value;
        }
        return hljs.highlightAuto(code).value;
    },
    breaks: true,
    gfm: true
});

// Connect to WebSocket
function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    updateConnectionStatus('connecting');

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        console.log('Connected to WebSocket');
        isConnected = true;
        reconnectAttempts = 0;
        reconnectDelay = 1000; // Reset delay
        updateConnectionStatus('connected');

        // Remove reconnection message if it exists
        removeReconnectionMessage();

        // Execute any pending actions from user interactions during disconnection
        if (window.pendingActions && window.pendingActions.length > 0) {
            console.log(`Executing ${window.pendingActions.length} pending actions`);
            const actions = [...window.pendingActions];
            window.pendingActions = [];
            
            // Show success message briefly
            if (actions.length > 0) {
                showSuccessMessage('Reconnected! Executing your action...');
            }
            
            // Execute all pending actions with a small delay between them
            actions.forEach((action, index) => {
                setTimeout(() => {
                    try {
                        action();
                    } catch (error) {
                        console.error('Error executing pending action:', error);
                    }
                }, index * 100);
            });
        } else {
            // Normal connection flow - load sessions and restore state
            loadSessions();

            // Try to restore previous session from localStorage
            const savedSessionId = localStorage.getItem('currentSessionId');
            if (savedSessionId) {
                loadSession(savedSessionId);
            } else {
                // Start a new session if none exists
                startNewSession();
            }
        }

        // Focus input when connected
        setTimeout(() => chatInput.focus(), 200);
    };

    ws.onmessage = (event) => {
        const message = JSON.parse(event.data);
        handleWebSocketMessage(message);
    };

    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        updateConnectionStatus('disconnected');
    };

    ws.onclose = () => {
        console.log('WebSocket connection closed');
        isConnected = false;
        updateConnectionStatus('disconnected');

        // Attempt to reconnect with exponential backoff
        if (reconnectAttempts < maxReconnectAttempts) {
            reconnectAttempts++;
            updateConnectionStatus('reconnecting');

            // Exponential backoff with jitter
            const delay = reconnectDelay * Math.pow(2, Math.min(reconnectAttempts - 1, 6)) + Math.random() * 1000;

            console.log(`Reconnecting in ${Math.round(delay)}ms (attempt ${reconnectAttempts}/${maxReconnectAttempts})`);

            setTimeout(() => {
                if (!isConnected) { // Only reconnect if still disconnected
                    connect();
                }
            }, delay);
        } else {
            updateConnectionStatus('failed');
            console.error('Max reconnection attempts reached');
        }
    };
}

// Handle incoming WebSocket messages
function handleWebSocketMessage(message) {
    switch (message.type) {
        case 'session_started':
            currentSessionId = message.session_id;
            // Save current session to localStorage
            localStorage.setItem('currentSessionId', currentSessionId);
            clearChat();
            loadSessions();
            // Update active session in UI
            setTimeout(() => updateActiveSession(currentSessionId), 100);
            // Focus input after starting new session
            setTimeout(() => chatInput.focus(), 100);
            break;

        case 'session_loaded':
            currentSessionId = message.session_id;
            // Save current session to localStorage
            localStorage.setItem('currentSessionId', currentSessionId);
            displayConversationHistory(message.data.history);
            // Update active session in UI
            updateActiveSession(currentSessionId);
            // Focus input after loading session
            setTimeout(() => chatInput.focus(), 100);
            break;

        case 'sessions_list':
            displaySessions(message.data.sessions);
            break;

        case 'status':
            showStatus(message.content);
            break;

        case 'response':
            if (message.streaming) {
                appendToStreamingMessage(message.content);
            } else {
                completeStreamingMessage(message.content);
            }
            break;

        case 'complete':
            hideStatus();
            // Auto-generate title after first response if this is a new conversation
            autoGenerateTitle();
            break;

        case 'title_generated':
            updateSessionTitle(message.session_id, message.data.title);
            break;

        case 'session_deleted':
            // Session deleted successfully, already removed from UI
            break;

        case 'error':
            showError(message.error);
            break;
    }
}

// Send a message
function sendMessage(content) {
    if (!content.trim() || isProcessing) {
        return;
    }

    // If not connected, try to reconnect first
    if (!isConnected) {
        reconnectAndExecute(() => sendMessage(content));
        return;
    }

    // If no current session, start new session first
    if (!currentSessionId) {
        startNewSession();
        // Queue message to send after session starts
        setTimeout(() => sendMessage(content), 500);
        return;
    }

    // Add user message to chat
    addMessage('user', content);

    // Clear input
    chatInput.value = '';
    adjustTextareaHeight();

    // Set processing state
    setProcessingState(true);

    // Send to server
    ws.send(JSON.stringify({
        type: 'message',
        content: content,
        session_id: currentSessionId
    }));
}

// Start a new session
function startNewSession() {
    // If not connected, try to reconnect first
    if (!isConnected) {
        reconnectAndExecute(() => startNewSession());
        return;
    }

    ws.send(JSON.stringify({
        type: 'start_session',
        data: {
            user_name: 'Santi',
            organization: 'TechClub Barcelona',
            role: 'developer',
            preferences: {
                timezone: 'Europe/Madrid',
                language: 'Spanish',
                theme: 'light'
            },
            metadata: {
                favorite_api: 'billing',
                experience_level: 'advanced'
            }
        }
    }));
}

// Load sessions list
function loadSessions() {
    if (!isConnected) return;

    ws.send(JSON.stringify({
        type: 'list_sessions'
    }));
}

// Update active session in UI
function updateActiveSession(sessionId) {
    // Remove active class from all sessions
    document.querySelectorAll('.session-item').forEach(item => {
        item.classList.remove('active');
    });

    // Add active class to the selected session
    const activeSession = document.querySelector(`[data-session-id="${sessionId}"]`);
    if (activeSession) {
        activeSession.classList.add('active');
    }
}

// Load a specific session
function loadSession(sessionId) {
    // If not connected, try to reconnect first
    if (!isConnected) {
        reconnectAndExecute(() => loadSession(sessionId));
        return;
    }

    currentSessionId = sessionId;

    // Update UI immediately
    updateActiveSession(sessionId);

    // Save to localStorage
    localStorage.setItem('currentSessionId', currentSessionId);

    ws.send(JSON.stringify({
        type: 'load_session',
        session_id: sessionId
    }));
}

// Display sessions in sidebar
function displaySessions(sessions) {
    sessionsList.innerHTML = '';

    if (sessions.length === 0) {
        sessionsList.innerHTML = '<div class="session-item">No sessions yet</div>';
        return;
    }

    sessions.forEach(session => {
        const sessionEl = document.createElement('div');
        sessionEl.className = 'session-item';
        sessionEl.dataset.sessionId = session.session_id;
        if (session.session_id === currentSessionId) {
            sessionEl.classList.add('active');
        }

        const date = new Date(session.created_at);
        const timeStr = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

        // Try to get saved title first, fallback to summary or default
        const savedTitle = getSavedTitle(session.session_id);
        const title = savedTitle || session.summary || 'New conversation';

        sessionEl.innerHTML = `
            <div class="session-content">
                <div style="font-weight: 600; margin-bottom: 0.25rem;">${timeStr}</div>
                <div class="session-title" style="font-size: 0.75rem; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" data-original-title="${title}">
                    ${title}
                </div>
            </div>
            <div class="session-actions">
                <button class="session-action-btn edit" title="Edit title" onclick="event.stopPropagation(); editSessionTitle('${session.session_id}')">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                        <path d="m12 19 7-7 3 3-7 7-3-3z"/>
                        <path d="m18 13-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/>
                        <path d="m2 2 7.586 7.586"/>
                        <circle cx="11" cy="11" r="2"/>
                    </svg>
                </button>
                <button class="session-action-btn delete" title="Delete conversation" onclick="event.stopPropagation(); deleteSession('${session.session_id}')">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                        <polyline points="3,6 5,6 21,6"/>
                        <path d="m19,6v14a2,2 0 0,1 -2,2H7a2,2 0 0,1 -2,-2V6m3,0V4a2,2 0 0,1 2,-2h4a2,2 0 0,1 2,2v2"/>
                        <line x1="10" y1="11" x2="10" y2="17"/>
                        <line x1="14" y1="11" x2="14" y2="17"/>
                    </svg>
                </button>
            </div>
        `;

        // Add click handler to session content only
        sessionEl.querySelector('.session-content').onclick = () => loadSession(session.session_id);
        sessionsList.appendChild(sessionEl);
    });
}

// Edit session title
function editSessionTitle(sessionId) {
    const sessionEl = document.querySelector(`[data-session-id="${sessionId}"]`);
    const titleEl = sessionEl.querySelector('.session-title');
    const currentTitle = titleEl.textContent;

    // Create input element
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'session-title-input';
    input.value = currentTitle;

    // Replace title with input
    titleEl.style.display = 'none';
    titleEl.parentNode.insertBefore(input, titleEl.nextSibling);
    input.focus();
    input.select();

    // Save function
    const saveTitle = () => {
        const newTitle = input.value.trim();
        if (newTitle && newTitle !== currentTitle) {
            saveSessionTitle(sessionId, newTitle);
            titleEl.textContent = newTitle;
            titleEl.dataset.originalTitle = newTitle;
        }

        // Restore original title display
        input.remove();
        titleEl.style.display = '';
    };

    // Event handlers
    input.onblur = saveTitle;
    input.onkeydown = (e) => {
        if (e.key === 'Enter') {
            saveTitle();
        } else if (e.key === 'Escape') {
            input.remove();
            titleEl.style.display = '';
        }
    };
}

// Delete session
function deleteSession(sessionId) {
    // If deleting current session, start a new one
    if (sessionId === currentSessionId) {
        startNewSession();
    }

    // Send delete request to server
    if (isConnected) {
        ws.send(JSON.stringify({
            type: 'delete_session',
            session_id: sessionId
        }));
    }

    // Remove from UI immediately
    const sessionEl = document.querySelector(`[data-session-id="${sessionId}"]`);
    if (sessionEl) {
        sessionEl.remove();
    }

    // Clean up localStorage
    removeSessionTitle(sessionId);
}

// Save session title to localStorage
function saveSessionTitle(sessionId, title) {
    const titles = JSON.parse(localStorage.getItem('sessionTitles') || '{}');
    titles[sessionId] = title;
    localStorage.setItem('sessionTitles', JSON.stringify(titles));
}

// Get saved session title from localStorage
function getSavedTitle(sessionId) {
    const titles = JSON.parse(localStorage.getItem('sessionTitles') || '{}');
    return titles[sessionId];
}

// Remove session title from localStorage
function removeSessionTitle(sessionId) {
    const titles = JSON.parse(localStorage.getItem('sessionTitles') || '{}');
    delete titles[sessionId];
    localStorage.setItem('sessionTitles', JSON.stringify(titles));
}

// Display conversation history
function displayConversationHistory(history) {
    clearChat();

    history.forEach(msg => {
        if (msg.role !== 'system') {
            addMessage(msg.role, msg.content, false);
        }
    });
}

// Add a message to the chat
function addMessage(role, content, animate = true) {
    // Ensure chat-content container exists
    let chatContent = chatMessages.querySelector('.chat-content');
    if (!chatContent) {
        chatContent = document.createElement('div');
        chatContent.className = 'chat-content';
        chatMessages.appendChild(chatContent);
    }

    // Remove welcome message if it exists
    const welcomeMsg = chatContent.querySelector('.welcome-message');
    if (welcomeMsg) {
        welcomeMsg.remove();
    }

    const messageEl = document.createElement('div');
    messageEl.className = `message ${role}`;

    const avatarText = role === 'user' ? 'U' : 'A';
    const roleText = role === 'user' ? 'You' : 'Assistant';

    messageEl.innerHTML = `
        <div class="message-avatar">${avatarText}</div>
        <div class="message-content">
            <div class="message-role">${roleText}</div>
            <div class="message-text">${marked.parse(content)}</div>
        </div>
    `;

    chatContent.appendChild(messageEl);

    if (animate) {
        messageEl.style.opacity = '0';
        messageEl.style.transform = 'translateY(10px)';
        setTimeout(() => {
            messageEl.style.transition = 'all 0.3s ease';
            messageEl.style.opacity = '1';
            messageEl.style.transform = 'translateY(0)';
        }, 10);
    }

    scrollToBottom();
}

// Handle streaming messages
let currentStreamingMessage = null;

function appendToStreamingMessage(content) {
    if (!currentStreamingMessage) {
        // Remove status indicator when starting actual response
        removeStatusIndicator();

        // Ensure chat-content container exists
        let chatContent = chatMessages.querySelector('.chat-content');
        if (!chatContent) {
            chatContent = document.createElement('div');
            chatContent.className = 'chat-content';
            chatMessages.appendChild(chatContent);
        }

        // Create new streaming message
        const welcomeMsg = chatContent.querySelector('.welcome-message');
        if (welcomeMsg) {
            welcomeMsg.remove();
        }

        const messageEl = document.createElement('div');
        messageEl.className = 'message assistant';
        messageEl.innerHTML = `
            <div class="message-avatar">A</div>
            <div class="message-content">
                <div class="message-role">Assistant <span class="streaming-indicator"></span></div>
                <div class="message-text"></div>
            </div>
        `;

        chatContent.appendChild(messageEl);
        currentStreamingMessage = messageEl.querySelector('.message-text');
        messageBuffer = '';
    }

    messageBuffer += content;
    currentStreamingMessage.innerHTML = marked.parse(messageBuffer);
    scrollToBottom();
}

function completeStreamingMessage(content) {
    if (currentStreamingMessage) {
        messageBuffer = content;
        currentStreamingMessage.innerHTML = marked.parse(messageBuffer);

        // Remove streaming indicator
        const indicator = currentStreamingMessage.closest('.message').querySelector('.streaming-indicator');
        if (indicator) {
            indicator.remove();
        }

        currentStreamingMessage = null;
        messageBuffer = '';
    } else {
        addMessage('assistant', content);
    }

    // Clear processing state
    setProcessingState(false);

    scrollToBottom();
}

// Show status message
function showStatus(message) {
    // Update status indicator text if it exists
    const statusEl = document.getElementById('status-indicator');
    if (statusEl) {
        const textSpan = statusEl.querySelector('span');
        if (textSpan) {
            textSpan.textContent = message;
        }
    }
}

function hideStatus() {
    // Status is handled by typing indicator now
}

// Show error message
function showError(error) {
    hideStatus();
    addMessage('system', `Error: ${error}`);

    // Clear processing state
    setProcessingState(false);
}

// Clear chat messages
function clearChat() {
    chatMessages.innerHTML = `
        <div class="chat-content">
            <div class="welcome-message">
                <h2>Welcome to V3 Agent Chat</h2>
                <p>Start a conversation by typing a message below.</p>
            </div>
        </div>
    `;
}

// Scroll to bottom of chat
function scrollToBottom() {
    chatMessages.scrollTop = chatMessages.scrollHeight;
}

// Auto-resize textarea
function adjustTextareaHeight() {
    chatInput.style.height = 'auto';
    chatInput.style.height = Math.min(chatInput.scrollHeight, 200) + 'px';
}

// Event listeners
chatForm.addEventListener('submit', (e) => {
    e.preventDefault();
    sendMessage(chatInput.value);
});

chatInput.addEventListener('input', adjustTextareaHeight);

chatInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage(chatInput.value);
    }
});

newChatBtn.addEventListener('click', () => {
    startNewSession();
    if (window.innerWidth <= 768) {
        sidebar.classList.remove('open');
    }
});

menuToggle.addEventListener('click', () => {
    sidebar.classList.toggle('open');
});

// Close sidebar when clicking outside on mobile
document.addEventListener('click', (e) => {
    if (window.innerWidth <= 768 &&
        !sidebar.contains(e.target) &&
        !menuToggle.contains(e.target) &&
        sidebar.classList.contains('open')) {
        sidebar.classList.remove('open');
    }
});

// Set processing state with visual feedback
function setProcessingState(processing) {
    isProcessing = processing;

    if (processing) {
        // Disable input and button
        chatInput.disabled = true;
        sendButton.disabled = true;

        // Change button appearance and icon
        sendButton.classList.add('processing');
        sendButton.innerHTML = `
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none" class="spinner">
                <circle cx="10" cy="10" r="8" stroke="currentColor" stroke-width="2" fill="none" stroke-dasharray="50.265" stroke-dashoffset="50.265">
                    <animate attributeName="stroke-dasharray" dur="1s" values="0 50.265;25.133 25.133;0 50.265" repeatCount="indefinite"/>
                    <animate attributeName="stroke-dashoffset" dur="1s" values="0;-25.133;-50.265" repeatCount="indefinite"/>
                </circle>
            </svg>
        `;

        // Add status indicator
        addStatusIndicator();

    } else {
        // Re-enable input and button
        chatInput.disabled = false;
        sendButton.disabled = false;
        chatInput.focus();

        // Restore button appearance and icon
        sendButton.classList.remove('processing');
        sendButton.innerHTML = `
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                <path d="M2 10l16-8-6 8 6 8-16-8zm6 0h6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
        `;

        // Remove status indicator
        removeStatusIndicator();
    }
}

// Add status indicator
function addStatusIndicator() {
    removeStatusIndicator(); // Remove any existing indicator

    // Ensure chat-content container exists
    let chatContent = chatMessages.querySelector('.chat-content');
    if (!chatContent) {
        chatContent = document.createElement('div');
        chatContent.className = 'chat-content';
        chatMessages.appendChild(chatContent);
    }

    const statusEl = document.createElement('div');
    statusEl.className = 'status-message';
    statusEl.id = 'status-indicator';
    statusEl.innerHTML = `
        <span>Processing request</span>
        <div class="typing-dots">
            <div class="typing-dot"></div>
            <div class="typing-dot"></div>
            <div class="typing-dot"></div>
        </div>
    `;

    chatContent.appendChild(statusEl);
    scrollToBottom();
}

// Remove status indicator
function removeStatusIndicator() {
    const statusEl = document.getElementById('status-indicator');
    if (statusEl) {
        statusEl.remove();
    }
}

// Auto-generate title for new conversations
function autoGenerateTitle() {
    if (!currentSessionId || !isConnected) return;

    // Check if we already have a title for this session
    const savedTitles = JSON.parse(localStorage.getItem('sessionTitles') || '{}');
    if (savedTitles[currentSessionId]) {
        return; // Already has a title
    }

    // Check if this is likely the first interaction (heuristic)
    const chatContent = document.querySelector('.chat-content');
    const messages = chatContent ? chatContent.querySelectorAll('.message') : [];
    const userMessages = Array.from(messages).filter(msg => msg.classList.contains('user'));

    // Generate title after first user message gets a response
    if (userMessages.length === 1) {
        ws.send(JSON.stringify({
            type: 'generate_title',
            session_id: currentSessionId
        }));
    }
}

// Update session title in UI and localStorage
function updateSessionTitle(sessionId, title) {
    // Save to localStorage
    const savedTitles = JSON.parse(localStorage.getItem('sessionTitles') || '{}');
    savedTitles[sessionId] = title;
    localStorage.setItem('sessionTitles', JSON.stringify(savedTitles));

    // Update UI if this session is in the sidebar
    const sessionElements = document.querySelectorAll('.session-item');
    sessionElements.forEach(el => {
        if (el.dataset.sessionId === sessionId) {
            const titleElement = el.querySelector('.session-title');
            if (titleElement) {
                titleElement.textContent = title;
            }
        }
    });
}

// Get saved title for session
function getSavedTitle(sessionId) {
    const savedTitles = JSON.parse(localStorage.getItem('sessionTitles') || '{}');
    return savedTitles[sessionId] || null;
}

// Update connection status display
function updateConnectionStatus(status) {
    connectionStatus = status;

    if (!connectionStatusEl) return;

    const dot = connectionStatusEl.querySelector('.connection-dot');
    const text = connectionStatusEl.querySelector('span');

    // Remove all status classes
    connectionStatusEl.classList.remove('visible', 'disconnected', 'connecting', 'reconnecting');

    switch (status) {
        case 'connected':
            text.textContent = 'Connected';
            // Hide status after 2 seconds when connected
            setTimeout(() => {
                if (connectionStatus === 'connected') {
                    connectionStatusEl.classList.remove('visible');
                }
            }, 2000);
            break;

        case 'connecting':
            connectionStatusEl.classList.add('visible', 'connecting');
            text.textContent = 'Connecting...';
            break;

        case 'reconnecting':
            connectionStatusEl.classList.add('visible', 'connecting');
            text.textContent = `Reconnecting... (${reconnectAttempts}/${maxReconnectAttempts})`;
            break;

        case 'reconnecting_action':
            connectionStatusEl.classList.add('visible', 'connecting');
            text.textContent = 'Reconnecting to execute your action...';
            break;

        case 'disconnected':
            connectionStatusEl.classList.add('visible', 'disconnected');
            text.textContent = 'Disconnected';
            break;

        case 'failed':
            connectionStatusEl.classList.add('visible', 'disconnected');
            text.textContent = 'Connection failed';
            break;
    }
}

// Retry connection manually
function retryConnection() {
    if (!isConnected && reconnectAttempts >= maxReconnectAttempts) {
        reconnectAttempts = 0;
        connect();
    }
}

// Reconnect and execute action on successful connection
function reconnectAndExecute(actionCallback) {
    // Reset reconnection attempts to allow fresh attempt
    reconnectAttempts = 0;
    
    // Store the action to execute after reconnection
    if (!window.pendingActions) {
        window.pendingActions = [];
    }
    window.pendingActions.push(actionCallback);
    
    // Show user that we're attempting to reconnect due to their action
    updateConnectionStatus('reconnecting_action');
    
    // Add temporary visual feedback in chat
    showReconnectionMessage();
    
    // Attempt reconnection
    if (!isConnected) {
        connect();
    }
}

// Show reconnection message in chat
function showReconnectionMessage() {
    const reconnectMsg = document.createElement('div');
    reconnectMsg.className = 'system-message reconnection-message';
    reconnectMsg.id = 'reconnection-indicator';
    reconnectMsg.innerHTML = `
        <div class="reconnection-content">
            <div class="reconnection-icon">🔄</div>
            <div class="reconnection-text">
                <div>Connection lost - attempting to reconnect...</div>
                <div class="reconnection-subtext">Your action will be executed once connected</div>
            </div>
        </div>
    `;
    
    // Ensure chat-content container exists
    let chatContent = chatMessages.querySelector('.chat-content');
    if (!chatContent) {
        chatContent = document.createElement('div');
        chatContent.className = 'chat-content';
        chatMessages.appendChild(chatContent);
    }
    
    chatContent.appendChild(reconnectMsg);
    scrollToBottom();
}

// Remove reconnection message
function removeReconnectionMessage() {
    const reconnectMsg = document.getElementById('reconnection-indicator');
    if (reconnectMsg) {
        reconnectMsg.remove();
    }
}

// Show success message briefly
function showSuccessMessage(message) {
    const successMsg = document.createElement('div');
    successMsg.className = 'system-message success-message';
    successMsg.innerHTML = `
        <div class="success-content">
            <div class="success-icon">✅</div>
            <div class="success-text">${message}</div>
        </div>
    `;
    
    // Ensure chat-content container exists
    let chatContent = chatMessages.querySelector('.chat-content');
    if (!chatContent) {
        chatContent = document.createElement('div');
        chatContent.className = 'chat-content';
        chatMessages.appendChild(chatContent);
    }
    
    chatContent.appendChild(successMsg);
    scrollToBottom();
    
    // Remove after 3 seconds
    setTimeout(() => {
        if (successMsg.parentNode) {
            successMsg.remove();
        }
    }, 3000);
}

// Add click handler for manual retry
if (connectionStatusEl) {
    connectionStatusEl.addEventListener('click', () => {
        if (connectionStatus === 'failed' || connectionStatus === 'disconnected') {
            retryConnection();
        }
    });
}

// Load chat configuration and initialize
async function loadChatConfig() {
    try {
        const response = await fetch('/api/config');
        const config = await response.json();

        // Update page title and header
        if (config.title) {
            document.title = config.title;
            const headerTitle = document.querySelector('.chat-header h1');
            if (headerTitle) {
                headerTitle.textContent = config.title;
            }
        }
    } catch (error) {
        console.error('Failed to load chat config:', error);
    }
}

// Initialize
loadChatConfig().then(() => {
    connect();
});