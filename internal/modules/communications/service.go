package communications

import (
	"context"
	"crypto/sha256"
	"fmt"
	"html"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"server_1/internal/core/config"
)

type EmailService struct {
	repo       *Repository
	provider   EmailProvider
	cfg        config.EmailConfig
}

func NewEmailService(cfg config.EmailConfig) *EmailService {
	_ = os.MkdirAll(cfg.AttachmentDir, 0o755)
	return &EmailService{
		repo:     NewRepository(),
		provider: NewSMTPProvider(cfg),
		cfg:      cfg,
	}
}

func (s *EmailService) CreateJob(ctx context.Context, req CreateEmailJobRequest) (*CreateEmailJobResponse, error) {
	if err := validateCreateRequest(req); err != nil {
		return nil, err
	}
	if existing, err := s.repo.FindJobByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
		return nil, err
	} else if existing != nil {
		return &CreateEmailJobResponse{
			JobID:         existing.ID,
			Status:        existing.Status,
			ReferenceType: existing.ReferenceType,
			ReferenceID:   existing.ReferenceID,
		}, nil
	}
	attachments, err := s.resolveAttachmentPaths(req.AttachmentPaths)
	if err != nil {
		return nil, err
	}
	bodyText := strings.TrimSpace(req.BodyText)
	if bodyText == "" {
		bodyText = htmlToText(req.BodyHTML)
	}
	jobID, err := s.repo.CreateJob(ctx, req, attachments, bodyText, nil)
	if err != nil {
		return nil, err
	}
	return &CreateEmailJobResponse{
		JobID:         jobID,
		Status:        StatusQueued,
		ReferenceType: req.ReferenceType,
		ReferenceID:   req.ReferenceID,
	}, nil
}

func validateCreateRequest(req CreateEmailJobRequest) error {
	if len(req.To) == 0 {
		return fmt.Errorf("at least one to recipient is required")
	}
	if strings.TrimSpace(req.Subject) == "" {
		return fmt.Errorf("subject is required")
	}
	if strings.TrimSpace(req.BodyHTML) == "" && strings.TrimSpace(req.BodyText) == "" {
		return fmt.Errorf("body_html or body_text is required")
	}
	if strings.TrimSpace(req.ReferenceType) == "" {
		return fmt.Errorf("reference_type is required")
	}
	if strings.TrimSpace(req.ReferenceID) == "" {
		return fmt.Errorf("reference_id is required")
	}
	for _, recipient := range append(append([]EmailRecipient{}, req.To...), append(req.CC, req.BCC...)...) {
		if !looksLikeEmail(recipient.Email) {
			return fmt.Errorf("invalid recipient email: %s", recipient.Email)
		}
	}
	for _, attachmentPath := range req.AttachmentPaths {
		if strings.TrimSpace(attachmentPath) == "" {
			return fmt.Errorf("attachment path cannot be empty")
		}
	}
	return nil
}

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.Contains(value, "@") && strings.Contains(value, ".")
}

func (s *EmailService) resolveAttachmentPaths(paths []string) ([]EmailAttachment, error) {
	attachments := make([]EmailAttachment, 0, len(paths))
	for _, rawPath := range paths {
		resolvedPath := strings.TrimSpace(rawPath)
		content, err := os.ReadFile(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("read attachment %s: %w", resolvedPath, err)
		}
		checksum := sha256.Sum256(content)
		fileName := filepath.Base(resolvedPath)
		mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
		if strings.TrimSpace(mimeType) == "" {
			mimeType = "application/octet-stream"
		}
		attachments = append(attachments, EmailAttachment{
			FileName:    fileName,
			MimeType:    mimeType,
			SizeBytes:   int64(len(content)),
			ChecksumSHA: fmt.Sprintf("%x", checksum[:]),
			StorageKey:  resolvedPath,
			SourceMode:  "path",
			SourcePath:  resolvedPath,
		})
	}
	return attachments, nil
}

func htmlToText(value string) string {
	re := regexp.MustCompile(`(?s)<[^>]*>`)
	decoded := html.UnescapeString(value)
	return strings.TrimSpace(re.ReplaceAllString(decoded, " "))
}

func (s *EmailService) GetJob(ctx context.Context, jobID int64) (*EmailJobDetailResponse, error) {
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return &EmailJobDetailResponse{Job: job}, nil
}

func (s *EmailService) ListLogs(ctx context.Context, filters EmailJobListFilters) (*EmailLogsResponse, error) {
	jobs, err := s.repo.ListJobSummaries(ctx, filters)
	if err != nil {
		return nil, err
	}
	return &EmailLogsResponse{Jobs: jobs}, nil
}

func (s *EmailService) RetryJob(ctx context.Context, jobID int64) (*RetryEmailJobResponse, error) {
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	newJobID, err := s.repo.CreateRetryJob(ctx, job)
	if err != nil {
		return nil, err
	}
	return &RetryEmailJobResponse{JobID: newJobID, Status: StatusQueued}, nil
}

func (s *EmailService) ProcessNextQueuedJob(ctx context.Context) (bool, error) {
	job, err := s.repo.ClaimNextQueuedJob(ctx, s.cfg.MaxAttempts)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}
	payload, err := s.buildProviderPayload(job)
	if err != nil {
		if completeErr := s.repo.CompleteAttempt(ctx, job, nil, err); completeErr != nil {
			return true, completeErr
		}
		return true, err
	}
	result, sendErr := s.provider.Send(payload)
	if err := s.repo.CompleteAttempt(ctx, job, result, sendErr); err != nil {
		return true, err
	}
	return true, sendErr
}

func (s *EmailService) buildProviderPayload(job *EmailJob) (providerPayload, error) {
	attachments := make([]providerAttachment, 0, len(job.Attachments))
	for _, attachment := range job.Attachments {
		content, err := os.ReadFile(attachment.StorageKey)
		if err != nil {
			return providerPayload{}, err
		}
		attachments = append(attachments, providerAttachment{
			FileName: attachment.FileName,
			MimeType: attachment.MimeType,
			Content:  content,
		})
	}
	return providerPayload{
		FromName:    s.cfg.FromName,
		FromEmail:   s.cfg.FromEmail,
		To:          parseRecipients(job.To),
		CC:          parseRecipients(job.CC),
		BCC:         parseRecipients(job.BCC),
		Subject:     job.Subject,
		BodyHTML:    job.BodyHTML,
		BodyText:    job.BodyText,
		Attachments: attachments,
	}, nil
}
