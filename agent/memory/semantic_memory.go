package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// SemanticFact represents a semantically structured piece of information
type SemanticFact struct {
	ID            string            `json:"id"`
	Content       string            `json:"content"`
	Category      string            `json:"category"`
	Entities      []string          `json:"entities"`      // Named entities extracted
	Keywords      []string          `json:"keywords"`      // Important keywords
	Relationships []Relationship    `json:"relationships"` // Connections to other facts
	Importance    float64           `json:"importance"`    // Calculated importance score
	Recency       float64           `json:"recency"`       // Recency score (decays over time)
	AccessCount   int               `json:"access_count"`  // How often this fact has been accessed
	LastAccessed  time.Time         `json:"last_accessed"`
	CreatedAt     time.Time         `json:"created_at"`
	Context       map[string]string `json:"context"` // Additional contextual information
}

// Relationship represents a connection between semantic facts
type Relationship struct {
	Type      string    `json:"type"`      // "related_to", "caused_by", "follows", etc.
	TargetID  string    `json:"target_id"` // ID of the related fact
	Strength  float64   `json:"strength"`  // Strength of the relationship (0-1)
	CreatedAt time.Time `json:"created_at"`
}

// SemanticMemory manages semantically structured conversation memory
type SemanticMemory struct {
	Facts               map[string]*SemanticFact `json:"facts"`
	TopicClusters       map[string][]string      `json:"topic_clusters"`       // Groups of related fact IDs
	ActiveContext       []string                 `json:"active_context"`       // Currently relevant fact IDs
	DecayRate           float64                  `json:"decay_rate"`           // How quickly recency decays
	ImportanceThreshold float64                  `json:"importance_threshold"` // Minimum importance to keep
	MaxFacts            int                      `json:"max_facts"`            // Maximum number of facts to store
}

// NewSemanticMemory creates a new semantic memory instance
func NewSemanticMemory() *SemanticMemory {
	return &SemanticMemory{
		Facts:               make(map[string]*SemanticFact),
		TopicClusters:       make(map[string][]string),
		ActiveContext:       make([]string, 0),
		DecayRate:           0.95, // 5% decay per day
		ImportanceThreshold: 0.1,
		MaxFacts:            500,
	}
}

// AddSemanticFact adds a new semantic fact to memory
func (sm *SemanticMemory) AddSemanticFact(content, category string, entities, keywords []string, importance float64, context map[string]string) *SemanticFact {
	id := generateFactID(content)

	// Check if fact already exists and update it
	if existing, exists := sm.Facts[id]; exists {
		existing.AccessCount++
		existing.LastAccessed = time.Now()
		existing.Recency = 1.0                                          // Reset recency for updated facts
		existing.Importance = math.Max(existing.Importance, importance) // Keep highest importance

		// Merge entities and keywords
		existing.Entities = mergeUniqueStrings(existing.Entities, entities)
		existing.Keywords = mergeUniqueStrings(existing.Keywords, keywords)

		// Update context
		for k, v := range context {
			existing.Context[k] = v
		}

		return existing
	}

	fact := &SemanticFact{
		ID:            id,
		Content:       content,
		Category:      category,
		Entities:      entities,
		Keywords:      keywords,
		Relationships: make([]Relationship, 0),
		Importance:    importance,
		Recency:       1.0,
		AccessCount:   1,
		LastAccessed:  time.Now(),
		CreatedAt:     time.Now(),
		Context:       context,
	}

	if fact.Context == nil {
		fact.Context = make(map[string]string)
	}

	sm.Facts[id] = fact

	// Find and create relationships with existing facts
	sm.createRelationships(fact)

	// Update topic clusters
	sm.updateTopicClusters(fact)

	// Clean up old facts if we exceed max
	if len(sm.Facts) > sm.MaxFacts {
		sm.cleanupOldFacts()
	}

	return fact
}

// GetRelevantFacts retrieves semantically relevant facts for a query
func (sm *SemanticMemory) GetRelevantFacts(query string, maxCount int) []*SemanticFact {
	if len(sm.Facts) == 0 {
		return []*SemanticFact{}
	}

	// Update recency scores for all facts
	sm.updateRecencyScores()

	queryLower := strings.ToLower(query)
	queryWords := extractKeywords(queryLower)

	type factScore struct {
		fact  *SemanticFact
		score float64
	}

	var scored []factScore

	for _, fact := range sm.Facts {
		score := sm.calculateRelevanceScore(fact, queryWords, queryLower)
		if score > 0 {
			scored = append(scored, factScore{fact, score})
		}
	}

	// Sort by relevance score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Return top facts and update their access information
	result := make([]*SemanticFact, 0, maxCount)
	for i, s := range scored {
		if i >= maxCount {
			break
		}

		// Update access information
		s.fact.AccessCount++
		s.fact.LastAccessed = time.Now()

		result = append(result, s.fact)
	}

	// Update active context with retrieved fact IDs
	sm.updateActiveContext(result)

	return result
}

// GetTopicCluster returns facts related to a specific topic
func (sm *SemanticMemory) GetTopicCluster(topic string) []*SemanticFact {
	factIDs, exists := sm.TopicClusters[topic]
	if !exists {
		return []*SemanticFact{}
	}

	facts := make([]*SemanticFact, 0, len(factIDs))
	for _, id := range factIDs {
		if fact, exists := sm.Facts[id]; exists {
			facts = append(facts, fact)
		}
	}

	return facts
}

// GetActiveContext returns the currently active context facts
func (sm *SemanticMemory) GetActiveContext() []*SemanticFact {
	facts := make([]*SemanticFact, 0, len(sm.ActiveContext))
	for _, id := range sm.ActiveContext {
		if fact, exists := sm.Facts[id]; exists {
			facts = append(facts, fact)
		}
	}
	return facts
}

// CompressToSummary creates a compressed summary while preserving key semantic information
func (sm *SemanticMemory) CompressToSummary(ctx context.Context, llmProvider llm.Provider) (string, error) {
	if len(sm.Facts) == 0 {
		return "", nil
	}

	// Get the most important facts
	importantFacts := sm.getMostImportantFacts(20)

	// Group facts by category for better organization
	categorized := make(map[string][]*SemanticFact)
	for _, fact := range importantFacts {
		categorized[fact.Category] = append(categorized[fact.Category], fact)
	}

	// Build structured summary
	var summaryParts []string
	for category, facts := range categorized {
		if len(facts) == 0 {
			continue
		}

		var factContents []string
		for _, fact := range facts {
			factContents = append(factContents, fact.Content)
		}

		categoryText := fmt.Sprintf("%s: %s", strings.Title(category), strings.Join(factContents, "; "))
		summaryParts = append(summaryParts, categoryText)
	}

	return strings.Join(summaryParts, "\n"), nil
}

// createRelationships finds and creates semantic relationships between facts
func (sm *SemanticMemory) createRelationships(newFact *SemanticFact) {
	for _, existingFact := range sm.Facts {
		if existingFact.ID == newFact.ID {
			continue
		}

		strength := sm.calculateRelationshipStrength(newFact, existingFact)
		if strength > 0.3 { // Threshold for meaningful relationships
			relType := sm.determineRelationshipType(newFact, existingFact)

			// Add bidirectional relationship
			newFact.Relationships = append(newFact.Relationships, Relationship{
				Type:      relType,
				TargetID:  existingFact.ID,
				Strength:  strength,
				CreatedAt: time.Now(),
			})

			existingFact.Relationships = append(existingFact.Relationships, Relationship{
				Type:      relType,
				TargetID:  newFact.ID,
				Strength:  strength,
				CreatedAt: time.Now(),
			})
		}
	}
}

// calculateRelationshipStrength determines how related two facts are
func (sm *SemanticMemory) calculateRelationshipStrength(fact1, fact2 *SemanticFact) float64 {
	strength := 0.0

	// Category similarity
	if fact1.Category == fact2.Category {
		strength += 0.3
	}

	// Entity overlap
	entityOverlap := calculateStringOverlap(fact1.Entities, fact2.Entities)
	strength += entityOverlap * 0.4

	// Keyword overlap
	keywordOverlap := calculateStringOverlap(fact1.Keywords, fact2.Keywords)
	strength += keywordOverlap * 0.3

	// Content similarity (simple word overlap)
	content1Words := extractKeywords(strings.ToLower(fact1.Content))
	content2Words := extractKeywords(strings.ToLower(fact2.Content))
	contentOverlap := calculateStringOverlap(content1Words, content2Words)
	strength += contentOverlap * 0.2

	return math.Min(strength, 1.0)
}

// determineRelationshipType determines the type of relationship between two facts
func (sm *SemanticMemory) determineRelationshipType(fact1, fact2 *SemanticFact) string {
	// Simple heuristics for relationship types
	if fact1.Category == fact2.Category {
		return "same_category"
	}

	// Time-based relationships
	if fact1.CreatedAt.After(fact2.CreatedAt) {
		return "follows"
	}

	return "related_to"
}

// calculateRelevanceScore calculates how relevant a fact is to a query
func (sm *SemanticMemory) calculateRelevanceScore(fact *SemanticFact, queryWords []string, queryLower string) float64 {
	score := 0.0

	// Content relevance
	contentLower := strings.ToLower(fact.Content)
	for _, word := range queryWords {
		if strings.Contains(contentLower, word) {
			score += 0.2
		}
	}

	// Entity relevance
	for _, entity := range fact.Entities {
		entityLower := strings.ToLower(entity)
		for _, word := range queryWords {
			if strings.Contains(entityLower, word) {
				score += 0.3
			}
		}
	}

	// Keyword relevance
	for _, keyword := range fact.Keywords {
		keywordLower := strings.ToLower(keyword)
		for _, word := range queryWords {
			if strings.Contains(keywordLower, word) {
				score += 0.25
			}
		}
	}

	// Category relevance
	categoryLower := strings.ToLower(fact.Category)
	for _, word := range queryWords {
		if strings.Contains(categoryLower, word) {
			score += 0.15
		}
	}

	// Weight by importance, recency, and access frequency
	score *= fact.Importance
	score *= fact.Recency
	score *= math.Log(float64(fact.AccessCount + 1)) // Logarithmic scaling for access count

	return score
}

// updateRecencyScores updates the recency scores for all facts based on time decay
func (sm *SemanticMemory) updateRecencyScores() {
	now := time.Now()
	for _, fact := range sm.Facts {
		daysSinceCreation := now.Sub(fact.CreatedAt).Hours() / 24
		fact.Recency = math.Pow(sm.DecayRate, daysSinceCreation)
	}
}

// updateTopicClusters updates topic clustering based on fact categories and relationships
func (sm *SemanticMemory) updateTopicClusters(fact *SemanticFact) {
	// Add to category cluster
	cluster := sm.TopicClusters[fact.Category]
	cluster = addUniqueString(cluster, fact.ID)
	sm.TopicClusters[fact.Category] = cluster

	// Create keyword-based clusters
	for _, keyword := range fact.Keywords {
		keywordCluster := sm.TopicClusters["keyword_"+keyword]
		keywordCluster = addUniqueString(keywordCluster, fact.ID)
		sm.TopicClusters["keyword_"+keyword] = keywordCluster
	}
}

// updateActiveContext updates the active context based on recently accessed facts
func (sm *SemanticMemory) updateActiveContext(accessedFacts []*SemanticFact) {
	// Clear old context
	sm.ActiveContext = make([]string, 0)

	// Add accessed facts to active context
	for _, fact := range accessedFacts {
		sm.ActiveContext = addUniqueString(sm.ActiveContext, fact.ID)
	}

	// Add related facts based on relationships
	for _, fact := range accessedFacts {
		for _, rel := range fact.Relationships {
			if rel.Strength > 0.5 { // High strength relationships
				sm.ActiveContext = addUniqueString(sm.ActiveContext, rel.TargetID)
			}
		}
	}

	// Limit active context size
	if len(sm.ActiveContext) > 20 {
		sm.ActiveContext = sm.ActiveContext[:20]
	}
}

// getMostImportantFacts returns the most important facts based on combined scoring
func (sm *SemanticMemory) getMostImportantFacts(count int) []*SemanticFact {
	type factScore struct {
		fact  *SemanticFact
		score float64
	}

	var scored []factScore
	for _, fact := range sm.Facts {
		// Combined score: importance * recency * log(access_count)
		score := fact.Importance * fact.Recency * math.Log(float64(fact.AccessCount+1))
		scored = append(scored, factScore{fact, score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]*SemanticFact, 0, count)
	for i, s := range scored {
		if i >= count {
			break
		}
		result = append(result, s.fact)
	}

	return result
}

// cleanupOldFacts removes the least important facts to maintain memory limits
func (sm *SemanticMemory) cleanupOldFacts() {
	if len(sm.Facts) <= sm.MaxFacts {
		return
	}

	type factScore struct {
		id    string
		score float64
	}

	var scored []factScore
	for id, fact := range sm.Facts {
		// Lower score = more likely to be removed
		score := fact.Importance * fact.Recency * math.Log(float64(fact.AccessCount+1))
		scored = append(scored, factScore{id, score})
	}

	// Sort by score (ascending - lowest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})

	// Remove lowest scoring facts
	toRemove := len(sm.Facts) - sm.MaxFacts + 50 // Remove extra to avoid frequent cleanup
	for i := 0; i < toRemove && i < len(scored); i++ {
		id := scored[i].id

		// Remove from Facts
		delete(sm.Facts, id)

		// Remove from topic clusters
		for topic, cluster := range sm.TopicClusters {
			newCluster := make([]string, 0)
			for _, factID := range cluster {
				if factID != id {
					newCluster = append(newCluster, factID)
				}
			}
			if len(newCluster) == 0 {
				delete(sm.TopicClusters, topic)
			} else {
				sm.TopicClusters[topic] = newCluster
			}
		}

		// Remove from active context
		newContext := make([]string, 0)
		for _, factID := range sm.ActiveContext {
			if factID != id {
				newContext = append(newContext, factID)
			}
		}
		sm.ActiveContext = newContext

		// Remove relationships pointing to this fact
		for _, fact := range sm.Facts {
			newRels := make([]Relationship, 0)
			for _, rel := range fact.Relationships {
				if rel.TargetID != id {
					newRels = append(newRels, rel)
				}
			}
			fact.Relationships = newRels
		}
	}
}

// Helper functions

func extractKeywords(text string) []string {
	// Simple keyword extraction - split by whitespace and filter
	words := strings.Fields(text)
	keywords := make([]string, 0)

	for _, word := range words {
		// Remove punctuation and filter short words
		word = strings.Trim(word, ".,!?;:")
		if len(word) > 2 {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

func calculateStringOverlap(slice1, slice2 []string) float64 {
	if len(slice1) == 0 || len(slice2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, s := range slice1 {
		set1[strings.ToLower(s)] = true
	}

	overlap := 0
	for _, s := range slice2 {
		if set1[strings.ToLower(s)] {
			overlap++
		}
	}

	maxLen := math.Max(float64(len(slice1)), float64(len(slice2)))
	return float64(overlap) / maxLen
}

func mergeUniqueStrings(slice1, slice2 []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)

	for _, s := range slice1 {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}

	for _, s := range slice2 {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}

	return result
}
