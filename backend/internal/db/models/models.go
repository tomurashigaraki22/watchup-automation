package models

import "time"

// Company is a discovered organization we may reach out to.
type Company struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	Website     string    `gorm:"uniqueIndex;size:512" json:"website"`
	Industry    string    `json:"industry"`
	Description string    `gorm:"type:text" json:"description"`
	Employees   string    `json:"employees"`
	Status      string    `gorm:"index;default:discovered" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Contact is a discovered email address belonging to a company.
type Contact struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	CompanyID         uint      `gorm:"index" json:"company_id"`
	Email             string    `gorm:"index;size:320" json:"email"`
	Name              string    `json:"name"`
	Source            string    `json:"source"`
	Priority          int       `gorm:"index" json:"priority"`
	Verified          bool      `json:"verified"`
	VerificationScore int       `json:"verification_score"`
	CreatedAt         time.Time `json:"created_at"`
}

// OutreachCampaign groups outreach with its own status and daily cap.
type OutreachCampaign struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `json:"name"`
	Status     string    `gorm:"index;default:active" json:"status"`
	DailyLimit int       `gorm:"default:25" json:"daily_limit"`
	SendMode   string    `gorm:"default:manual" json:"send_mode"`
	CreatedAt  time.Time `json:"created_at"`
}

// Email is a single message (original or followup) and its lifecycle state.
type Email struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	CampaignID uint       `gorm:"index" json:"campaign_id"`
	CompanyID  uint       `gorm:"index" json:"company_id"`
	ContactID  uint       `gorm:"index" json:"contact_id"`
	Subject    string     `json:"subject"`
	Body       string     `gorm:"type:text" json:"body"`
	BodyText   string     `gorm:"type:text" json:"body_text"`
	MessageID  string     `gorm:"index;size:512" json:"message_id"`
	Status     string     `gorm:"index;default:draft" json:"status"`
	Opened     bool       `json:"opened"`
	Clicked    bool       `json:"clicked"`
	Replied    bool       `gorm:"index" json:"replied"`
	Bounced    bool       `json:"bounced"`
	SMTPResp   string     `gorm:"type:text" json:"smtp_response"`
	SentAt     *time.Time `json:"sent_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Followup is a scheduled follow-up message in a sequence.
type Followup struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	EmailID     uint       `gorm:"index" json:"email_id"`
	Sequence    int        `json:"sequence"`
	ScheduledAt time.Time  `gorm:"index" json:"scheduled_at"`
	Sent        bool       `gorm:"index" json:"sent"`
	SentAt      *time.Time `json:"sent_at"`
	Canceled    bool       `gorm:"index" json:"canceled"`
}

// AIGeneration records every Gemini call for auditing and cost tracking.
type AIGeneration struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CompanyID uint      `gorm:"index" json:"company_id"`
	Kind      string    `json:"kind"` // analysis | email | followup
	Prompt    string    `gorm:"type:text" json:"prompt"`
	Response  string    `gorm:"type:text" json:"response"`
	Model     string    `json:"model"`
	Tokens    int       `json:"tokens"`
	CreatedAt time.Time `json:"created_at"`
}

// Suppression is a permanently do-not-contact email/domain.
type Suppression struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Email     string    `gorm:"uniqueIndex;size:320" json:"email"`
	Reason    string    `json:"reason"` // unsubscribe | hard_bounce | manual
	CreatedAt time.Time `json:"created_at"`
}

// AuditLog is a structured record of every side-effecting action.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Actor     string    `json:"actor"` // system | worker | user:<id>
	Action    string    `gorm:"index" json:"action"`
	Entity    string    `json:"entity"`
	EntityID  uint      `json:"entity_id"`
	Detail    string    `gorm:"type:text" json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// All returns every model for AutoMigrate.
func All() []any {
	return []any{
		&Company{},
		&Contact{},
		&OutreachCampaign{},
		&Email{},
		&Followup{},
		&AIGeneration{},
		&Suppression{},
		&AuditLog{},
	}
}
