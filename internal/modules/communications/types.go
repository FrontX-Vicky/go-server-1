package communications

import "time"

const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSent       = "sent"
	StatusFailed     = "failed"
)

type EmailRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type TriggeredBy struct {
	UserID string `json:"user_id,omitempty"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
}

type CreateEmailJobRequest struct {
	To              []EmailRecipient `json:"to"`
	CC              []EmailRecipient `json:"cc,omitempty"`
	BCC             []EmailRecipient `json:"bcc,omitempty"`
	Subject         string           `json:"subject"`
	BodyHTML        string           `json:"body_html"`
	BodyText        string           `json:"body_text,omitempty"`
	AttachmentPaths []string         `json:"attachment_paths,omitempty"`
	ReferenceType   string           `json:"reference_type"`
	ReferenceID     string           `json:"reference_id"`
	ReferenceLabel  string           `json:"reference_label,omitempty"`
	ModuleKey       string           `json:"module_key,omitempty"`
	SenderKey       string           `json:"sender_key,omitempty"`
	TriggeredBy     *TriggeredBy     `json:"triggered_by,omitempty"`
	IdempotencyKey  string           `json:"idempotency_key,omitempty"`
}

type EmailJobListFilters struct {
	Status        string
	ModuleKey     string
	ReferenceType string
	ReferenceID   string
	Recipient     string
	Actor         string
	DateFrom      string
	DateTo        string
	Limit         int
	Offset        int
}

type EmailAttachment struct {
	ID           int64     `json:"id"`
	JobID        int64     `json:"job_id"`
	FileName     string    `json:"file_name"`
	MimeType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size_bytes"`
	ChecksumSHA  string    `json:"checksum_sha256"`
	StorageKey   string    `json:"storage_key"`
	SourceMode   string    `json:"source_mode"`
	SourcePath   string    `json:"source_path,omitempty"`
	SourceURL    string    `json:"source_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type EmailAttemptLog struct {
	ID                int64      `json:"id"`
	JobID             int64      `json:"job_id"`
	AttemptNo         int        `json:"attempt_no"`
	Status            string     `json:"status"`
	ProviderMessageID string     `json:"provider_message_id,omitempty"`
	ProviderResponse  string     `json:"provider_response,omitempty"`
	ErrorClass        string     `json:"error_class,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	Subject           string     `json:"subject"`
	To                string     `json:"to"`
	CC                string     `json:"cc"`
	BCC               string     `json:"bcc"`
	StartedAt         time.Time  `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type EmailJob struct {
	ID                 int64            `json:"id"`
	Status             string           `json:"status"`
	ReferenceType      string           `json:"reference_type"`
	ReferenceID        string           `json:"reference_id"`
	ReferenceLabel     string           `json:"reference_label"`
	ModuleKey          string           `json:"module_key"`
	SenderKey          string           `json:"sender_key"`
	Subject            string           `json:"subject"`
	BodyHTML           string           `json:"body_html"`
	BodyText           string           `json:"body_text"`
	To                 string           `json:"to"`
	CC                 string           `json:"cc"`
	BCC                string           `json:"bcc"`
	TriggeredByUserID  string           `json:"triggered_by_user_id"`
	TriggeredByName    string           `json:"triggered_by_name"`
	TriggeredByEmail   string           `json:"triggered_by_email"`
	IdempotencyKey     string           `json:"idempotency_key"`
	RetryOfJobID       *int64           `json:"retry_of_job_id,omitempty"`
	LatestAttemptID    *int64           `json:"latest_attempt_id,omitempty"`
	LatestError        string           `json:"latest_error"`
	QueuedAt           time.Time        `json:"queued_at"`
	ProcessingStartedAt *time.Time      `json:"processing_started_at,omitempty"`
	SentAt             *time.Time       `json:"sent_at,omitempty"`
	FailedAt           *time.Time       `json:"failed_at,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	Attachments        []EmailAttachment `json:"attachments,omitempty"`
	Attempts           []EmailAttemptLog `json:"attempts,omitempty"`
}

type EmailJobSummary struct {
	ID              int64      `json:"id"`
	Status          string     `json:"status"`
	ReferenceType   string     `json:"reference_type"`
	ReferenceID     string     `json:"reference_id"`
	ReferenceLabel  string     `json:"reference_label"`
	ModuleKey       string     `json:"module_key"`
	Subject         string     `json:"subject"`
	PrimaryTo       string     `json:"primary_to"`
	TriggeredByName string     `json:"triggered_by_name"`
	TriggeredByEmail string    `json:"triggered_by_email"`
	LatestError     string     `json:"latest_error"`
	RetryOfJobID    *int64     `json:"retry_of_job_id,omitempty"`
	QueuedAt        time.Time  `json:"queued_at"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	FailedAt        *time.Time `json:"failed_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type EmailJobDetailResponse struct {
	Job *EmailJob `json:"job"`
}

type EmailLogsResponse struct {
	Jobs []EmailJobSummary `json:"jobs"`
}

type RetryEmailJobResponse struct {
	JobID  int64  `json:"job_id"`
	Status string `json:"status"`
}

type CreateEmailJobResponse struct {
	JobID         int64  `json:"job_id"`
	Status        string `json:"status"`
	ReferenceType string `json:"reference_type"`
	ReferenceID   string `json:"reference_id"`
}

// SenderInfo is the public-safe representation of a configured sender account.
type SenderInfo struct {
	Key       string `json:"key"`
	FromEmail string `json:"from_email"`
	FromName  string `json:"from_name"`
}

type ListSendersResponse struct {
	Senders []SenderInfo `json:"senders"`
}

type providerPayload struct {
	FromName    string
	FromEmail   string
	To          []EmailRecipient
	CC          []EmailRecipient
	BCC         []EmailRecipient
	Subject     string
	BodyHTML    string
	BodyText    string
	Attachments []providerAttachment
}

type providerAttachment struct {
	FileName string
	MimeType string
	Content  []byte
}

type providerResult struct {
	MessageID string
	Response  string
}
