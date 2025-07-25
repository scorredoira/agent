# 🛠️ Principios de Desarrollo - V3 Agent  

## 🚫 **Regla de Oro: NO HARDCODING**

**NUNCA implementes lógica hardcodeada sin consultar primero si hay una alternativa mejor.**

### ❌ **Evitar Siempre:**
- Keywords lists para detección de intenciones
- Patrones fijos de texto para análisis
- Lógica if/else compleja para decisiones
- Mapeos estáticos de consulta → acción
- Configuraciones fijas que podrían ser dinámicas

### ✅ **Enfoques Preferidos:**
1. **LLM-Based Decision Making**: Que el LLM analice y decida
2. **Function Calling**: Usar capacidades nativas de los LLM providers
3. **Dynamic Configuration**: Configuración flexible y extensible
4. **Plugin Architecture**: Sistemas extensibles por diseño
5. **Self-Describing Systems**: Que los componentes se describan a sí mismos

## 🧠 **Arquitectura de Herramientas**

### **Nuevo Enfoque: LLM-Driven Tool Selection**

```
Usuario consulta → LLM analiza contexto → LLM solicita herramientas → Agente ejecuta → LLM continúa
```

#### **Implementación:**
1. **System Prompt Dinámico**: 
   - Lista automática de herramientas disponibles
   - Descripciones generadas desde las herramientas mismas
   - Contexto actualizado en tiempo real

2. **Function Calling Nativo**:
   - OpenAI: `tools` parameter con `function` objects
   - Anthropic: `tools` parameter con tool definitions  
   - Gemini: `tools` con `function_declarations`

3. **Tool Registry Auto-descriptivo**:
   - Cada herramienta se describe a sí misma
   - Schemas generados automáticamente
   - Capabilities detectadas dinámicamente

### **Ejemplo de Flujo:**

```go
// ❌ MALO - Hardcoded
if strings.Contains(query, "authentication") {
    useKnowledgeBase = true
}

// ✅ BUENO - LLM decides
systemPrompt := buildDynamicSystemPrompt(toolRegistry)
llmResponse := llm.Complete(systemPrompt + userQuery)
if llmResponse.wantsToUseTool("kbase") {
    executeTool("kbase", llmResponse.toolParams)
}
```

## 📋 **Proceso de Decisión**

### **Antes de implementar, pregúntate:**

1. **¿Puede el LLM decidir esto mejor que código hardcodeado?** → Probablemente SÍ
2. **¿Esta lógica será flexible para casos futuros?** → Si no, replantearlo
3. **¿Estoy asumiendo patrones que podrían cambiar?** → Hacerlo dinámico
4. **¿Hay una forma más declarativa de expresar esto?** → Usarla

### **Cuando Consultar:**
- **Siempre** antes de escribir listas de keywords
- **Siempre** antes de lógica if/else compleja para decisiones
- **Siempre** antes de mappings estáticos
- Cuando sientes que estás "adivinando" el comportamiento del usuario

## 🎯 **Objetivos de Diseño**

### **Flexibilidad**
- El sistema debe adaptarse a nuevos casos sin cambios de código
- Las herramientas deben ser plug-and-play
- Las decisiones deben basarse en contexto, no reglas fijas

### **Auto-descripción**
- Componentes que se explican a sí mismos
- Schemas generados automáticamente
- Documentación que emerge del código

### **LLM-Native**
- Aprovechar las capacidades de razonamiento del LLM
- Function calling como primitiva básica
- Context-aware decision making

## 🚀 **Implementación Inmediata**

### **Tarea Actual: Tool Selection Refactor**

1. **Eliminar TaskPlanner keyword-based**
2. **Implementar Function Calling** para OpenAI/Anthropic/Gemini
3. **System Prompt Dinámico** con herramientas disponibles
4. **Auto-registration** de herramientas como functions
5. **LLM-driven execution** flow

### **Código de Ejemplo:**

```go
// Tool auto-describes itself for LLM
type Tool interface {
    GetFunctionDefinition() *FunctionDefinition
    GetDescription() string
    GetParameterSchema() *JSONSchema
}

// System prompt builds itself
func (a *Agent) buildSystemPrompt() string {
    tools := a.registry.GetAvailableTools()
    return fmt.Sprintf(`
You are V3 Agent with access to these tools:
%s

Use tools when needed to provide accurate responses.
`, tools.ToFunctionDefinitions())
}
```

## 💡 **Recordatorio**

**"If you find yourself writing hardcoded logic, stop and ask: Can the LLM do this better?"**

La respuesta casi siempre es SÍ. 🧠✨