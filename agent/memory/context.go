package memory

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// SystemContextProvider proporciona contexto del sistema al agente
type SystemContextProvider struct {
	startTime time.Time
	version   string
}

// NewSystemContextProvider crea un nuevo proveedor de contexto del sistema
func NewSystemContextProvider(version string) *SystemContextProvider {
	return &SystemContextProvider{
		startTime: time.Now(),
		version:   version,
	}
}

// GetSystemContext devuelve el contexto del sistema actual
func (scp *SystemContextProvider) GetSystemContext() llm.Message {
	now := time.Now()
	
	context := fmt.Sprintf(`SYSTEM CONTEXT - Current Information:

Date & Time: %s
Timezone: %s
Day of week: %s
System: %s %s
Agent Version: %s
Session Start: %s
Uptime: %s

IMPORTANT: You are a club management AI assistant. Always use the current date/time information above when users ask about dates, times, or scheduling.`,
		now.Format("Monday, January 2, 2006 at 3:04 PM"),
		now.Format("MST"),
		now.Weekday().String(),
		runtime.GOOS,
		runtime.GOARCH,
		scp.version,
		scp.startTime.Format("3:04 PM"),
		time.Since(scp.startTime).Truncate(time.Second).String(),
	)
	
	return llm.Message{
		Role:    "system",
		Content: context,
	}
}

// GetDateTimeContext devuelve contexto específico de fecha/hora
func (scp *SystemContextProvider) GetDateTimeContext() string {
	now := time.Now()
	return fmt.Sprintf("Current date and time: %s (%s)", 
		now.Format("Monday, January 2, 2006 at 3:04 PM"), 
		now.Format("MST"))
}

// GetBusinessContext devuelve contexto específico para gestión de clubes
func (scp *SystemContextProvider) GetBusinessContext() llm.Message {
	return llm.Message{
		Role: "system",
		Content: `You are a specialized AI assistant for sports and social club management. Your expertise includes:

- Membership management and pricing strategies
- Facility booking and scheduling systems  
- Event planning and coordination
- Financial management for clubs
- Member communication and retention
- Compliance with sports regulations
- Technology integration for club operations

Always provide specific, actionable advice tailored to club management needs.`,
	}
}

// EnhanceContextWithSystem añade contexto del sistema a mensajes existentes
func (scp *SystemContextProvider) EnhanceContextWithSystem(messages []llm.Message, includeDateTime bool) []llm.Message {
	enhanced := make([]llm.Message, 0, len(messages)+2)
	
	// Añadir contexto de negocio si no hay mensajes del sistema
	hasSystemMessage := false
	for _, msg := range messages {
		if msg.Role == "system" {
			hasSystemMessage = true
			break
		}
	}
	
	if !hasSystemMessage {
		enhanced = append(enhanced, scp.GetBusinessContext())
	}
	
	// Añadir contexto de fecha/hora si es solicitado
	if includeDateTime {
		enhanced = append(enhanced, scp.GetSystemContext())
	}
	
	// Añadir mensajes originales
	enhanced = append(enhanced, messages...)
	
	return enhanced
}

// ShouldIncludeDateTime determina si debe incluirse contexto de fecha/hora
func (scp *SystemContextProvider) ShouldIncludeDateTime(query string) bool {
	dateTimeKeywords := []string{
		"fecha", "date", "dia", "day", "hoy", "today", "ahora", "now",
		"cuando", "when", "hora", "time", "horario", "schedule",
		"calendario", "calendar", "mañana", "tomorrow", "ayer", "yesterday",
		"semana", "week", "mes", "month", "año", "year",
	}
	
	queryLower := strings.ToLower(query)
	for _, keyword := range dateTimeKeywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	
	return false
}