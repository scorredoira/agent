# Conversation Title Generation Prompt

Generate a short, descriptive title (maximum 5 words) for a conversation that starts with this user message: "{{.FirstUserMessage}}"

The title should:
- Be very concise (3-5 words maximum)
- Capture the main topic or intent
- Be in the same language as the user message
- Not include quotes or special characters
- Be suitable as a conversation title

Examples:
- "¿Cómo crear clientes?" → "Crear clientes API"
- "How to implement login?" → "Login implementation help"
- "Explain React hooks" → "React hooks explanation"
- "dame ejemplos de curl" → "Ejemplos curl API"
- "autenticación API" → "Autenticación API"

Title: