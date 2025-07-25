# Agent Chat Server

Un servidor web tipo ChatGPT que integra la librería agent para crear un chatbot interactivo con interfaz web moderna.

## Características

- 🚀 **WebSocket en tiempo real**: Comunicación bidireccional para respuestas inmediatas
- 💬 **Interfaz tipo ChatGPT**: UI moderna y familiar con soporte para markdown
- 🧠 **Sesiones persistentes**: Mantiene el historial de conversaciones
- 🎨 **Diseño responsive**: Funciona en desktop y móvil
- ⚡ **Streaming de respuestas**: Muestra las respuestas mientras se generan

## Instalación

```bash
# Desde el directorio cmd/chat
go build -o chat .
```

## Uso

### Ejecutar el servidor

```bash
# Con configuración por defecto
./chat

# Con puerto personalizado
./chat -port 3000

# Con archivo de configuración específico
./chat -config ../../config.json

# Con directorio de almacenamiento personalizado
./chat -storage ./my_conversations
```

### Acceder a la interfaz

Abre tu navegador en `http://localhost:8080` (o el puerto que hayas configurado).

## Integración en tu SaaS

La librería agent está diseñada para ser fácilmente integrable. El método clave es `ServeWebSocket`:

```go
// En tu aplicación
import "github.com/santiagocorredoira/agent/agent"

// Crear el agente
agentConfig := agent.AgentConfig{
    ConfigPath:    "config.json",
    StorageDir:    "./conversations",
    ToolsOnlyMode: false,
}

agentInstance, err := agent.NewV3Agent(agentConfig)
if err != nil {
    log.Fatal(err)
}

// En tu router HTTP
http.HandleFunc("/ws", agentInstance.ServeWebSocket)
```

### Personalización del contexto

Puedes añadir contexto específico del usuario al iniciar una sesión:

```javascript
// En el cliente JavaScript
ws.send(JSON.stringify({
    type: 'start_session',
    data: {
        user_name: 'Juan',
        organization: 'Mi Empresa',
        role: 'admin',
        preferences: {
            language: 'es',
            timezone: 'Europe/Madrid'
        }
    }
}));
```

## Protocolo WebSocket

### Mensajes del cliente al servidor

1. **Iniciar sesión**:
```json
{
    "type": "start_session",
    "data": {
        "user_name": "string",
        "organization": "string",
        "role": "string",
        "preferences": {}
    }
}
```

2. **Enviar mensaje**:
```json
{
    "type": "message",
    "content": "Tu pregunta aquí",
    "session_id": "session_123"
}
```

3. **Cargar sesión**:
```json
{
    "type": "load_session",
    "session_id": "session_123"
}
```

4. **Listar sesiones**:
```json
{
    "type": "list_sessions"
}
```

### Mensajes del servidor al cliente

1. **Sesión iniciada**:
```json
{
    "type": "session_started",
    "session_id": "session_123"
}
```

2. **Respuesta (streaming)**:
```json
{
    "type": "response",
    "content": "Parte de la respuesta...",
    "session_id": "session_123",
    "streaming": true
}
```

3. **Respuesta completa**:
```json
{
    "type": "complete",
    "session_id": "session_123",
    "data": {
        "tokens": {
            "prompt": 100,
            "completion": 50,
            "total": 150
        }
    }
}
```

4. **Error**:
```json
{
    "type": "error",
    "error": "Descripción del error"
}
```

## Estructura del proyecto

```
cmd/chat/
├── main.go          # Servidor HTTP mínimo
├── static/          
│   ├── index.html   # UI del chat
│   ├── styles.css   # Estilos dark mode
│   └── app.js       # Cliente WebSocket
└── README.md        # Este archivo
```

## Notas de implementación

- El servidor usa Gorilla WebSocket para manejar las conexiones
- Las sesiones se almacenan en disco en formato JSON
- El frontend usa Marked.js para renderizar markdown
- El código está resaltado con Highlight.js

## Próximos pasos

Para integrar en tu SaaS:

1. **Autenticación**: Añade middleware de autenticación antes del WebSocket upgrade
2. **Base de datos**: Modifica el almacenamiento para usar tu BD en lugar de archivos
3. **Herramientas personalizadas**: Registra tools específicas de tu negocio
4. **Contexto del negocio**: Añade proveedores de contexto con información de tu dominio
5. **Límites de uso**: Implementa rate limiting y cuotas por usuario

## Ejemplo de integración completa

```go
// middleware/auth.go
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Verificar JWT token
        user := getUserFromToken(r)
        if user == nil {
            http.Error(w, "Unauthorized", 401)
            return
        }
        
        // Añadir usuario al contexto
        ctx := context.WithValue(r.Context(), "user", user)
        next.ServeHTTP(w, r.WithContext(ctx))
    }
}

// main.go
http.HandleFunc("/ws", AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
    user := r.Context().Value("user").(User)
    
    // Crear agente con contexto del usuario
    agent.SetUserContext(user.ID, user.Name, user.Organization)
    agent.ServeWebSocket(w, r)
}))
```