package models

// Company lifecycle — advances left to right as the pipeline processes it.
const (
	CompanyStatusDiscovered = "discovered"
	CompanyStatusCrawled    = "crawled"
	CompanyStatusValidated  = "validated"
	CompanyStatusAnalyzed   = "analyzed"
	CompanyStatusQueued     = "queued"
	CompanyStatusContacted  = "contacted"
	CompanyStatusFailed     = "failed"
)

// Email lifecycle.
const (
	EmailStatusDraft   = "draft"
	EmailStatusQueued  = "queued"
	EmailStatusSent    = "sent"
	EmailStatusFailed  = "failed"
	EmailStatusBounced = "bounced"
	EmailStatusReplied = "replied"
)

// Campaign status.
const (
	CampaignStatusActive  = "active"
	CampaignStatusPaused  = "paused"
	CampaignStatusDeleted = "deleted"
)

// Send mode — whether AI-generated emails require human approval.
const (
	SendModeManual    = "manual"
	SendModeAutomatic = "automatic"
)

// AIGeneration kind — what the Gemini call was for.
const (
	AIGenerationKindAnalysis = "analysis"
	AIGenerationKindEmail    = "email"
	AIGenerationKindFollowup = "followup"
)

// Suppression reason.
const (
	SuppressionReasonUnsubscribe = "unsubscribe"
	SuppressionReasonHardBounce  = "hard_bounce"
	SuppressionReasonManual      = "manual"
)
