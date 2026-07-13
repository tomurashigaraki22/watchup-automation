package queue

// JobType identifies which pipeline stage a job represents.
type JobType string

const (
	JobDiscover  JobType = "discover"
	JobCrawl     JobType = "crawl"
	JobValidate  JobType = "validate"
	JobAnalyze   JobType = "analyze"
	JobGenerate  JobType = "generate"
	JobSend      JobType = "send"
	JobFollowup  JobType = "followup"
	JobReplyScan JobType = "reply_scan"
)

// Job is one unit of pipeline work. Not every field applies to every
// JobType — see the workers package for which fields each handler reads.
type Job struct {
	Type       JobType `json:"type"`
	CompanyID  uint    `json:"company_id,omitempty"`
	EmailID    uint    `json:"email_id,omitempty"`
	CampaignID uint    `json:"campaign_id,omitempty"`
	FollowupID uint    `json:"followup_id,omitempty"`
}
