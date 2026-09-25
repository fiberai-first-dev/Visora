package db

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Types for JSONB fields
type StringArray []string

type Project struct {
	ID          uint      `gorm:"primaryKey" json:"-"`
	PublicID    string    `gorm:"uniqueIndex;size:16" json:"id"`
	Name        string    `gorm:"not null"`
	Brand       string    `gorm:"not null"`
	Website     string    `gorm:"not null"`
	Category    string    `gorm:"not null"`
	Country     string    `gorm:"not null;default:'India'"`
	UserEmail   string    `gorm:"index"`
	Competitors string    `gorm:"type:jsonb;not null;default:'[]'"`
	ProfileData string    `gorm:"type:jsonb;default:'{}'"`
	Status      string    `gorm:"not null;default:'active'"`
	CreatedAt   time.Time `gorm:"not null;default:now()"`
}

type CrawlRun struct {
	ID              uint      `gorm:"primaryKey"`
	ProjectID       uint      `gorm:"index;not null"`
	CompetitorID    *uint     `gorm:"index"` // set when this run crawled a competitor domain
	Status          string    `gorm:"not null;default:'queued'"`
	Trigger         string    `gorm:"not null;default:'manual'"`
	PagesDiscovered int       `gorm:"not null;default:0"`
	PagesCrawled    int       `gorm:"not null;default:0"`
	MaxPages        int       `gorm:"not null;default:50"`
	Error           *string   
	Stats           string    `gorm:"type:jsonb;default:'{}'"`
	StartedAt       *time.Time
	FinishedAt      *time.Time
	CreatedAt       time.Time `gorm:"not null;default:now()"`
}

type Page struct {
	ID                  uint      `gorm:"primaryKey"`
	ProjectID           uint      `gorm:"index;not null"`
	CrawlRunID          uint      `gorm:"index;not null"`
	CompetitorID        *uint     `gorm:"index"` // set when the page belongs to a competitor site
	URL                 string    `gorm:"not null"`
	PageType            string    `gorm:"not null;default:'other'"`
	Status              *int
	ContentType         *string
	Title               *string
	MetaDescription     *string
	H1                  string    `gorm:"type:jsonb;not null;default:'[]'"`
	H2                  string    `gorm:"type:jsonb;not null;default:'[]'"`
	H3                  string    `gorm:"type:jsonb;not null;default:'[]'"`
	Canonical           *string
	RobotsMeta          *string
	WordCount           int       `gorm:"not null;default:0"`
	// BodyText is stripped main copy (capped) so AI can suggest exact wording changes.
	BodyText            string    `gorm:"type:text;not null;default:''"`
	// FaqJson is extracted FAQ Q&A pairs for GEO/SEO rewrite suggestions.
	FaqJson             string    `gorm:"type:jsonb;not null;default:'[]'"`
	Images              int       `gorm:"not null;default:0"`
	ImagesMissingAlt    int       `gorm:"not null;default:0"`
	InternalLinks       int       `gorm:"not null;default:0"`
	ExternalLinks       int       `gorm:"not null;default:0"`
	StructuredDataTypes string    `gorm:"type:jsonb;not null;default:'[]'"`
	IsProductPage       bool      `gorm:"not null;default:false"`
	HasProductSchema    bool      `gorm:"not null;default:false"`
	Price               *string
	Availability        *string
	HasReviews          bool      `gorm:"not null;default:false"`
	HasFaq              bool      `gorm:"not null;default:false"`
	OgTitle             *string
	OgDescription       *string
	OgImage             *string
	Depth               int       `gorm:"not null;default:0"`
	Error               *string
	// ResponseMs and ResponseBytes are measured during the crawl, so the
	// performance screen can report real numbers instead of estimates.
	ResponseMs    int       `gorm:"not null;default:0"`
	ResponseBytes int64     `gorm:"not null;default:0"`
	FetchedAt     time.Time `gorm:"not null;default:now()"`
}

type SeoIssue struct {
	ID             uint      `gorm:"primaryKey"`
	ProjectID      uint      `gorm:"index;not null"`
	CrawlRunID     uint      `gorm:"not null"`
	PageID         *uint     
	URL            *string
	RuleID         string    `gorm:"not null"`
	Severity       string    `gorm:"not null"`
	Category       string    `gorm:"not null"`
	Title          string    `gorm:"not null"`
	Detail         string    `gorm:"not null"`
	Recommendation string    `gorm:"not null"`
	Data           string    `gorm:"type:jsonb;default:'{}'"`
	CreatedAt      time.Time `gorm:"not null;default:now()"`
}

type Keyword struct {
	ID            uint      `gorm:"primaryKey"`
	ProjectID     uint      `gorm:"index;not null"`
	Keyword       string    `gorm:"not null"`
	Intent        string    `gorm:"not null"`
	Source        string    `gorm:"not null;default:'generated'"`
	MatchedPageID *uint
	Status        string    `gorm:"not null;default:'opportunity'"`
	Note          *string
	CreatedAt     time.Time `gorm:"not null;default:now()"`
}

type GeoPrompt struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	Text      string    `gorm:"not null"`
	Intent    string    `gorm:"not null;default:'commercial'"`
	Topic     *string
	Active    bool      `gorm:"not null;default:true"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

type GeoRun struct {
	ID            uint      `gorm:"primaryKey"`
	ProjectID     uint      `gorm:"index;not null"`
	Status        string    `gorm:"not null;default:'queued'"`
	Model         string    `gorm:"not null"`
	PromptsTotal  int       `gorm:"not null;default:0"`
	PromptsDone   int       `gorm:"not null;default:0"`
	PromptsFailed int       `gorm:"not null;default:0"`
	Error         *string
	StartedAt     *time.Time
	FinishedAt    *time.Time
	CreatedAt     time.Time `gorm:"not null;default:now()"`
}

type GeoResponse struct {
	ID           uint      `gorm:"primaryKey"`
	ProjectID    uint      `gorm:"index;not null"`
	RunID        uint      `gorm:"index;not null"`
	PromptID     uint      `gorm:"not null"`
	Model        string    `gorm:"not null"`
	ResponseText string    `gorm:"not null"`
	Analysis     string    `gorm:"type:jsonb;default:'{}'"`
	CreatedAt    time.Time `gorm:"not null;default:now()"`
}

type GeoMention struct {
	ID         uint    `gorm:"primaryKey"`
	ProjectID  uint    `gorm:"index;not null"`
	RunID      uint    `gorm:"index;not null"`
	ResponseID uint    `gorm:"not null"`
	Brand      string  `gorm:"not null"`
	IsTarget   bool    `gorm:"not null"`
	Mentioned  bool    `gorm:"not null"`
	Position   *int
	Context    *string
	Sentiment  string  `gorm:"not null;default:'neutral'"`
}

type GeoCitation struct {
	ID            uint   `gorm:"primaryKey"`
	ProjectID     uint   `gorm:"index;not null"`
	RunID         uint   `gorm:"index;not null"`
	ResponseID    uint   `gorm:"not null"`
	URL           string `gorm:"not null"`
	Domain        string `gorm:"not null"`
	SourceType    string `gorm:"not null;default:'other'"`
	IsBrandDomain bool   `gorm:"not null;default:false"`
}

type Recommendation struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	Source    string    `gorm:"not null"`
	Severity  string    `gorm:"not null"`
	Title     string    `gorm:"not null"`
	Detail    string    `gorm:"not null"`
	Action    string    `gorm:"not null"`
	Status    string    `gorm:"not null;default:'open'"`
	Data      string    `gorm:"type:jsonb;default:'{}'"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

type Job struct {
	ID         uint      `gorm:"primaryKey"`
	Type       string    `gorm:"not null"`
	Payload    string    `gorm:"type:jsonb;not null"`
	Status     string    `gorm:"index;not null;default:'queued'"`
	Attempts   int       `gorm:"not null;default:0"`
	Error      *string
	Result     string    `gorm:"type:jsonb;default:'{}'"`
	StartedAt  *time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time `gorm:"not null;default:now()"`
}

type ProjectMetric struct {
	ID            uint      `gorm:"primaryKey"`
	ProjectID     uint      `gorm:"index;not null"`
	SeoScore      *float32
	GeoScore      *float32
	MentionRate   *float32
	CitationRate  *float32
	CompetitorSov *float32
	ComputedAt    time.Time `gorm:"not null;default:now()"`
	Breakdown     string    `gorm:"type:jsonb;default:'{}'"`
}

// ---------------------------------------------------------------- missing parts from PRD

type Organization struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

type User struct {
	ID             uint      `gorm:"primaryKey"`
	OrganizationID uint      `gorm:"index"`
	Email          string    `gorm:"unique;not null"`
	Name           string
	CreatedAt      time.Time `gorm:"not null;default:now()"`
}

type Website struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	URL       string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

type Brand struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"not null"`
	Domain    string
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

type BrandCategory struct {
	ID      uint   `gorm:"primaryKey"`
	BrandID uint   `gorm:"index;not null"`
	Name    string `gorm:"not null"`
}

type BrandProduct struct {
	ID      uint   `gorm:"primaryKey"`
	BrandID uint   `gorm:"index;not null"`
	Name    string `gorm:"not null"`
	Price   *string
}

type PageSnapshot struct {
	ID         uint      `gorm:"primaryKey"`
	PageID     uint      `gorm:"index;not null"`
	ContentHash string   `gorm:"not null"`
	HTML        string
	CreatedAt   time.Time `gorm:"not null;default:now()"`
}

type Topic struct {
	ID        uint   `gorm:"primaryKey"`
	ProjectID uint   `gorm:"index;not null"`
	Name      string `gorm:"not null"`
}

type SearchQuery struct {
	ID        uint   `gorm:"primaryKey"`
	ProjectID uint   `gorm:"index;not null"`
	Query     string `gorm:"not null"`
	Volume    *int
}

type CompetitorCandidate struct {
	ID               uint       `gorm:"primaryKey"`
	ProjectID        uint       `gorm:"index;not null"`
	Domain           string     `gorm:"not null"`
	Status           string     `gorm:"default:'pending'"`
	BrandName        string
	Classification   string     `gorm:"default:'unknown'"` // direct_d2c | search | content | marketplace | publisher | unknown
	Appearances      int        `gorm:"not null;default:0"`
	BestPosition     *int
	SharedQueryCount int        `gorm:"not null;default:0"`
	CrawlStatus      string     `gorm:"default:'pending'"` // pending | crawled | skipped
	CrawledAt        *time.Time
	PagesCrawled     int        `gorm:"not null;default:0"`
}

type CompetitorRelationship struct {
	ID           uint   `gorm:"primaryKey"`
	ProjectID    uint   `gorm:"index;not null"`
	CompetitorID uint   `gorm:"index;not null"`
	Type         string `gorm:"not null"` // direct, search, content
}

type ContentOpportunity struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	Topic     string    `gorm:"not null"`
	Intent    string
	Status    string    `gorm:"default:'open'"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

type ChangeEvent struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	EventType string    `gorm:"not null"`
	Detail    string    `gorm:"type:jsonb;default:'{}'"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

type Integration struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	Provider  string    `gorm:"not null"`
	Config    string    `gorm:"type:jsonb;default:'{}'"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

// SearchIntent — output of step 4
type SearchIntent struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	Keyword   string    `gorm:"not null"`
	Intent    string    `gorm:"not null"` // informational | commercial | transactional | navigational
	Source    string    `gorm:"not null"` // product | category | topic | entity | comparison
	Priority  int       `gorm:"not null;default:0"` // higher = more important
	Status    string    `gorm:"not null;default:'opportunity'"` // opportunity | covered
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

// SerpResult — output of step 5
type SerpResult struct {
	ID          uint      `gorm:"primaryKey"`
	ProjectID   uint      `gorm:"index;not null"`
	IntentID    uint      `gorm:"index;not null"`
	Query       string    `gorm:"not null"`
	Domain      string    `gorm:"not null"`
	Position    int       `gorm:"not null"`
	PageTitle   string
	PageURL     string    `gorm:"not null"`
	PageSnippet string
	IsOwnDomain bool      `gorm:"not null;default:false"`
	CheckedAt   time.Time `gorm:"not null;default:now()"`
}

// KeywordGap — output of step 8
type KeywordGap struct {
	ID                     uint      `gorm:"primaryKey"`
	ProjectID              uint      `gorm:"index;not null"`
	Query                  string    `gorm:"not null"`
	BrandPosition          *int      // null means not ranking
	BestCompetitor         string
	BestCompetitorPosition *int
	GapType                string    `gorm:"not null"` // missing | lagging
	CreatedAt              time.Time `gorm:"not null;default:now()"`
}

// ContentGap — output of step 8
type ContentGap struct {
	ID                   uint      `gorm:"primaryKey"`
	ProjectID            uint      `gorm:"index;not null"`
	Topic                string    `gorm:"not null"`
	Intent               string    `gorm:"not null"`
	CompetitorDomains    string    `gorm:"type:jsonb;not null;default:'[]'"` // JSON array of domains
	EvidenceQueries      string    `gorm:"type:jsonb;not null;default:'[]'"` // JSON array of query strings
	ExistingRelatedPages string    `gorm:"type:jsonb;not null;default:'[]'"` // JSON array of {url, title}
	Priority             string    `gorm:"not null;default:'medium'"` // critical | high | medium | low
	Status               string    `gorm:"not null;default:'open'"` // open | dismissed
	CreatedAt            time.Time `gorm:"not null;default:now()"`
}

// ScanEvent — for SSE stream
//
// The JSON tags are part of the streaming contract with the frontend scan UI and
// must not change without updating frontend/src/components/scan/EventStream.tsx.
type ScanEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"index;not null" json:"project_id"`
	Stage     int       `gorm:"not null" json:"stage"` // 1-16
	EventType string    `gorm:"not null" json:"event_type"` // progress | milestone | error | complete
	Message   string    `gorm:"not null" json:"message"`
	Data      RawJSON   `gorm:"type:jsonb;default:'{}'" json:"data"`
	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
}

// RawJSON is a JSON document stored as text that is emitted as real JSON when it
// is serialized to an API response. It lets handlers pass through stored JSON
// blobs without double encoding them.
type RawJSON string

// MarshalJSON writes the stored document verbatim, falling back to an empty object.
func (r RawJSON) MarshalJSON() ([]byte, error) {
	trimmed := strings.TrimSpace(string(r))
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return []byte("{}"), nil
	}
	return []byte(trimmed), nil
}

// UnmarshalJSON keeps the raw bytes so the document survives a round trip.
func (r *RawJSON) UnmarshalJSON(b []byte) error {
	*r = RawJSON(b)
	return nil
}

// MonitorEvent — step 15-16, ongoing monitoring
type MonitorEvent struct {
	ID        uint      `gorm:"primaryKey"`
	ProjectID uint      `gorm:"index;not null"`
	EventType string    `gorm:"not null"` // ranking_change | new_competitor | technical_regression | content_change
	Title     string    `gorm:"not null"`
	Detail    string    `gorm:"type:jsonb;default:'{}'"`
	Severity  string    `gorm:"not null;default:'notice'"` // critical | warning | notice
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

// GeneratedFix — step 13
type GeneratedFix struct {
	ID                 uint      `gorm:"primaryKey"`
	ProjectID          uint      `gorm:"index;not null"`
	RecommendationID   *uint
	FixType            string    `gorm:"not null"` // content | meta | schema | faq | geo
	PageURL            string
	Title              string    `gorm:"not null"`
	Content            string    `gorm:"type:text;not null"` // the actual generated content
	Status             string    `gorm:"not null;default:'pending'"` // pending | applied | exported
	CreatedAt          time.Time `gorm:"not null;default:now()"`
}

// ProjectMetricsHistory — step 16 improvement tracking
type ProjectMetricsHistory struct {
	ID              uint      `gorm:"primaryKey"`
	ProjectID       uint      `gorm:"index;not null"`
	SeoScore        *float32
	KeywordsRanking int
	TopTenCount     int
	CompetitorCount int
	ComputedAt      time.Time `gorm:"not null;default:now()"`
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Project{},
		&CrawlRun{},
		&Page{},
		&SeoIssue{},
		&Keyword{},
		&GeoPrompt{},
		&GeoRun{},
		&GeoResponse{},
		&GeoMention{},
		&GeoCitation{},
		&Recommendation{},
		&Job{},
		&ProjectMetric{},
		&Organization{},
		&User{},
		&Website{},
		&Brand{},
		&BrandCategory{},
		&BrandProduct{},
		&PageSnapshot{},
		&Topic{},
		&SearchQuery{},
		&CompetitorCandidate{},
		&CompetitorRelationship{},
		&ContentOpportunity{},
		&ChangeEvent{},
		&Integration{},
		&SearchIntent{},
		&SerpResult{},
		&KeywordGap{},
		&ContentGap{},
		&ScanEvent{},
		&MonitorEvent{},
		&GeneratedFix{},
		&ProjectMetricsHistory{},
	)
}
