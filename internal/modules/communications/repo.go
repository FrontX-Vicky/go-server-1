package communications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	coredb "server_1/internal/core/db"
)

var ensureSchemaOnce sync.Once

type Repository struct {
	db *coredb.SQL
}

func NewRepository() *Repository {
	repo := &Repository{db: coredb.DBx("DB1")}
	ensureSchemaOnce.Do(func() {
		if err := repo.ensureSchema(context.Background()); err != nil {
			panic(fmt.Sprintf("communications schema init failed: %v", err))
		}
	})
	return repo
}

func (r *Repository) ensureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS email_jobs (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			status VARCHAR(32) NOT NULL,
			reference_type VARCHAR(120) NOT NULL,
			reference_id VARCHAR(191) NOT NULL,
			reference_label VARCHAR(255) NOT NULL DEFAULT '',
			module_key VARCHAR(120) NOT NULL DEFAULT '',
			subject TEXT NOT NULL,
			body_html LONGTEXT NOT NULL,
			body_text LONGTEXT NOT NULL,
			to_json LONGTEXT NOT NULL,
			cc_json LONGTEXT NOT NULL,
			bcc_json LONGTEXT NOT NULL,
			triggered_by_user_id VARCHAR(120) NOT NULL DEFAULT '',
			triggered_by_name VARCHAR(255) NOT NULL DEFAULT '',
			triggered_by_email VARCHAR(255) NOT NULL DEFAULT '',
			idempotency_key VARCHAR(191) NULL DEFAULT NULL,
			retry_of_job_id BIGINT NULL,
			latest_attempt_id BIGINT NULL,
			latest_error TEXT NULL,
			queued_at DATETIME NOT NULL,
			processing_started_at DATETIME NULL,
			sent_at DATETIME NULL,
			failed_at DATETIME NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE KEY uq_email_jobs_idempotency (idempotency_key),
			KEY idx_email_jobs_status (status, queued_at),
			KEY idx_email_jobs_reference (reference_type, reference_id, id),
			KEY idx_email_jobs_module (module_key, id),
			KEY idx_email_jobs_retry (retry_of_job_id)
		)`,
		`CREATE TABLE IF NOT EXISTS email_attempt_logs (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			job_id BIGINT NOT NULL,
			attempt_no INT NOT NULL,
			status VARCHAR(32) NOT NULL,
			provider_message_id VARCHAR(255) NOT NULL DEFAULT '',
			provider_response TEXT NULL,
			error_class VARCHAR(120) NOT NULL DEFAULT '',
			error_message TEXT NULL,
			subject TEXT NOT NULL,
			to_json LONGTEXT NOT NULL,
			cc_json LONGTEXT NOT NULL,
			bcc_json LONGTEXT NOT NULL,
			started_at DATETIME NOT NULL,
			finished_at DATETIME NULL,
			created_at DATETIME NOT NULL,
			KEY idx_email_attempt_logs_job (job_id, id),
			KEY idx_email_attempt_logs_status (status, created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS email_attachments (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			job_id BIGINT NOT NULL,
			file_name VARCHAR(255) NOT NULL,
			mime_type VARCHAR(255) NOT NULL,
			size_bytes BIGINT NOT NULL DEFAULT 0,
			checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
			storage_key VARCHAR(255) NOT NULL,
			source_mode VARCHAR(32) NOT NULL DEFAULT 'uploaded',
			source_url TEXT NULL,
			created_at DATETIME NOT NULL,
			KEY idx_email_attachments_job (job_id, id)
		)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	// Safe column additions — ignore "Duplicate column name" so this is idempotent.
	alterStatements := []string{
		`ALTER TABLE email_jobs ADD COLUMN sender_key VARCHAR(120) NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alterStatements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			if !strings.Contains(err.Error(), "Duplicate column name") {
				return err
			}
		}
	}
	return nil
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func parseRecipients(raw string) []EmailRecipient {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var recipients []EmailRecipient
	if err := json.Unmarshal([]byte(raw), &recipients); err != nil {
		return nil
	}
	return recipients
}

func primaryRecipient(raw string) string {
	recipients := parseRecipients(raw)
	if len(recipients) == 0 {
		return ""
	}
	if recipients[0].Name != "" {
		return fmt.Sprintf("%s <%s>", recipients[0].Name, recipients[0].Email)
	}
	return recipients[0].Email
}

func (r *Repository) FindJobByIdempotencyKey(ctx context.Context, key string) (*EmailJob, error) {
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	row := r.db.QueryRowContext(ctx, `SELECT
		id, status, reference_type, reference_id, reference_label, module_key, COALESCE(sender_key,''),
		subject, body_html, body_text, to_json, cc_json, bcc_json,
		COALESCE(triggered_by_user_id,''), COALESCE(triggered_by_name,''), COALESCE(triggered_by_email,''),
		COALESCE(idempotency_key,''), retry_of_job_id, latest_attempt_id, COALESCE(latest_error,''),
		queued_at, processing_started_at, sent_at, failed_at, created_at, updated_at
		FROM email_jobs WHERE idempotency_key = ? LIMIT 1`, key)
	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

func (r *Repository) CreateJob(ctx context.Context, req CreateEmailJobRequest, attachments []EmailAttachment, bodyText string, retryOfJobID *int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	triggeredBy := &TriggeredBy{}
	if req.TriggeredBy != nil {
		triggeredBy = req.TriggeredBy
	}

	var idempotencyKey any
	if strings.TrimSpace(req.IdempotencyKey) != "" {
		idempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	}

	result, err := tx.ExecContext(ctx, `INSERT INTO email_jobs (
		status, reference_type, reference_id, reference_label, module_key, sender_key,
		subject, body_html, body_text, to_json, cc_json, bcc_json,
		triggered_by_user_id, triggered_by_name, triggered_by_email,
		idempotency_key, retry_of_job_id, latest_error,
		queued_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		StatusQueued, req.ReferenceType, req.ReferenceID, req.ReferenceLabel, req.ModuleKey, req.SenderKey,
		req.Subject, req.BodyHTML, bodyText, mustJSON(req.To), mustJSON(req.CC), mustJSON(req.BCC),
		triggeredBy.UserID, triggeredBy.Name, triggeredBy.Email,
		idempotencyKey, retryOfJobID, "", now, now, now,
	)
	if err != nil {
		return 0, err
	}
	jobID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, attachment := range attachments {
		if _, err := tx.ExecContext(ctx, `INSERT INTO email_attachments (
			job_id, file_name, mime_type, size_bytes, checksum_sha256, storage_key, source_mode, source_url, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			jobID, attachment.FileName, attachment.MimeType, attachment.SizeBytes, attachment.ChecksumSHA,
			attachment.StorageKey, attachment.SourceMode, attachment.SourcePath, now,
		); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return jobID, nil
}

func (r *Repository) GetJob(ctx context.Context, jobID int64) (*EmailJob, error) {
	row := r.db.QueryRowContext(ctx, `SELECT
		id, status, reference_type, reference_id, reference_label, module_key, COALESCE(sender_key,''),
		subject, body_html, body_text, to_json, cc_json, bcc_json,
		COALESCE(triggered_by_user_id,''), COALESCE(triggered_by_name,''), COALESCE(triggered_by_email,''),
		COALESCE(idempotency_key,''), retry_of_job_id, latest_attempt_id, COALESCE(latest_error,''),
		queued_at, processing_started_at, sent_at, failed_at, created_at, updated_at
		FROM email_jobs WHERE id = ? LIMIT 1`, jobID)
	job, err := scanJob(row)
	if err != nil {
		return nil, err
	}
	job.Attachments, err = r.ListAttachments(ctx, jobID)
	if err != nil {
		return nil, err
	}
	job.Attempts, err = r.ListAttempts(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func scanJob(scanner interface {
	Scan(dest ...any) error
}) (*EmailJob, error) {
	job := &EmailJob{}
	var retryOf sql.NullInt64
	var latestAttempt sql.NullInt64
	var processingAt sql.NullTime
	var sentAt sql.NullTime
	var failedAt sql.NullTime
	if err := scanner.Scan(
		&job.ID, &job.Status, &job.ReferenceType, &job.ReferenceID, &job.ReferenceLabel, &job.ModuleKey, &job.SenderKey,
		&job.Subject, &job.BodyHTML, &job.BodyText, &job.To, &job.CC, &job.BCC,
		&job.TriggeredByUserID, &job.TriggeredByName, &job.TriggeredByEmail,
		&job.IdempotencyKey, &retryOf, &latestAttempt, &job.LatestError,
		&job.QueuedAt, &processingAt, &sentAt, &failedAt, &job.CreatedAt, &job.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if retryOf.Valid {
		job.RetryOfJobID = &retryOf.Int64
	}
	if latestAttempt.Valid {
		job.LatestAttemptID = &latestAttempt.Int64
	}
	if processingAt.Valid {
		value := processingAt.Time
		job.ProcessingStartedAt = &value
	}
	if sentAt.Valid {
		value := sentAt.Time
		job.SentAt = &value
	}
	if failedAt.Valid {
		value := failedAt.Time
		job.FailedAt = &value
	}
	return job, nil
}

func (r *Repository) ListAttachments(ctx context.Context, jobID int64) ([]EmailAttachment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, job_id, file_name, mime_type, size_bytes,
		checksum_sha256, storage_key, source_mode, COALESCE(source_url,''), created_at
		FROM email_attachments WHERE job_id = ? ORDER BY id ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var attachments []EmailAttachment
	for rows.Next() {
		var attachment EmailAttachment
		if err := rows.Scan(
			&attachment.ID, &attachment.JobID, &attachment.FileName, &attachment.MimeType, &attachment.SizeBytes,
			&attachment.ChecksumSHA, &attachment.StorageKey, &attachment.SourceMode, &attachment.SourceURL, &attachment.CreatedAt,
		); err != nil {
			return nil, err
		}
		attachment.SourcePath = attachment.SourceURL
		attachments = append(attachments, attachment)
	}
	return attachments, rows.Err()
}

func (r *Repository) ListAttempts(ctx context.Context, jobID int64) ([]EmailAttemptLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, job_id, attempt_no, status, provider_message_id,
		COALESCE(provider_response,''), COALESCE(error_class,''), COALESCE(error_message,''),
		subject, to_json, cc_json, bcc_json, started_at, finished_at, created_at
		FROM email_attempt_logs WHERE job_id = ? ORDER BY id DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var attempts []EmailAttemptLog
	for rows.Next() {
		var attempt EmailAttemptLog
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&attempt.ID, &attempt.JobID, &attempt.AttemptNo, &attempt.Status, &attempt.ProviderMessageID,
			&attempt.ProviderResponse, &attempt.ErrorClass, &attempt.ErrorMessage,
			&attempt.Subject, &attempt.To, &attempt.CC, &attempt.BCC, &attempt.StartedAt, &finishedAt, &attempt.CreatedAt,
		); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			value := finishedAt.Time
			attempt.FinishedAt = &value
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (r *Repository) ListJobSummaries(ctx context.Context, filters EmailJobListFilters) ([]EmailJobSummary, error) {
	where := []string{"1=1"}
	args := make([]any, 0, 10)
	if filters.Status != "" {
		where = append(where, "status = ?")
		args = append(args, filters.Status)
	}
	if filters.ModuleKey != "" {
		where = append(where, "module_key = ?")
		args = append(args, filters.ModuleKey)
	}
	if filters.ReferenceType != "" {
		where = append(where, "reference_type = ?")
		args = append(args, filters.ReferenceType)
	}
	if filters.ReferenceID != "" {
		where = append(where, "(reference_id = ? OR FIND_IN_SET(?, reference_id) > 0)")
		args = append(args, filters.ReferenceID, filters.ReferenceID)
	}
	if filters.DateFrom != "" {
		where = append(where, "DATE(created_at) >= ?")
		args = append(args, filters.DateFrom)
	}
	if filters.DateTo != "" {
		where = append(where, "DATE(created_at) <= ?")
		args = append(args, filters.DateTo)
	}
	if filters.Recipient != "" {
		where = append(where, "(to_json LIKE ? OR cc_json LIKE ? OR bcc_json LIKE ?)")
		needle := "%" + filters.Recipient + "%"
		args = append(args, needle, needle, needle)
	}
	if filters.Actor != "" {
		where = append(where, "(triggered_by_name LIKE ? OR triggered_by_email LIKE ?)")
		needle := "%" + filters.Actor + "%"
		args = append(args, needle, needle)
	}
	limit := filters.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	query := `SELECT id, status, reference_type, reference_id, reference_label, module_key,
		subject, to_json, triggered_by_name, triggered_by_email, COALESCE(latest_error,''),
		retry_of_job_id, queued_at, sent_at, failed_at, updated_at
		FROM email_jobs WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []EmailJobSummary
	for rows.Next() {
		var job EmailJobSummary
		var toJSON string
		var retryOf sql.NullInt64
		var sentAt sql.NullTime
		var failedAt sql.NullTime
		if err := rows.Scan(
			&job.ID, &job.Status, &job.ReferenceType, &job.ReferenceID, &job.ReferenceLabel, &job.ModuleKey,
			&job.Subject, &toJSON, &job.TriggeredByName, &job.TriggeredByEmail, &job.LatestError,
			&retryOf, &job.QueuedAt, &sentAt, &failedAt, &job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		job.PrimaryTo = primaryRecipient(toJSON)
		if retryOf.Valid {
			job.RetryOfJobID = &retryOf.Int64
		}
		if sentAt.Valid {
			value := sentAt.Time
			job.SentAt = &value
		}
		if failedAt.Valid {
			value := failedAt.Time
			job.FailedAt = &value
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Repository) CreateRetryJob(ctx context.Context, source *EmailJob) (int64, error) {
	req := CreateEmailJobRequest{
		To:             parseRecipients(source.To),
		CC:             parseRecipients(source.CC),
		BCC:            parseRecipients(source.BCC),
		Subject:        source.Subject,
		BodyHTML:       source.BodyHTML,
		BodyText:       source.BodyText,
		ReferenceType:  source.ReferenceType,
		ReferenceID:    source.ReferenceID,
		ReferenceLabel: source.ReferenceLabel,
		ModuleKey:      source.ModuleKey,
		SenderKey:      source.SenderKey,
		TriggeredBy: &TriggeredBy{
			UserID: source.TriggeredByUserID,
			Name:   source.TriggeredByName,
			Email:  source.TriggeredByEmail,
		},
	}
	attachments := make([]EmailAttachment, len(source.Attachments))
	copy(attachments, source.Attachments)
	retryOf := source.ID
	return r.CreateJob(ctx, req, attachments, source.BodyText, &retryOf)
}

func (r *Repository) ClaimNextQueuedJob(ctx context.Context, maxAttempts int) (*EmailJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `SELECT j.id
		FROM email_jobs j
		LEFT JOIN (
			SELECT job_id, COUNT(*) AS attempt_count
			FROM email_attempt_logs
			GROUP BY job_id
		) a ON a.job_id = j.id
		WHERE j.status = ? AND COALESCE(a.attempt_count, 0) < ?
		ORDER BY j.id ASC
		LIMIT 1
		FOR UPDATE`, StatusQueued, maxAttempts)
	var jobID int64
	if err := row.Scan(&jobID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE email_jobs
		SET status = ?, processing_started_at = ?, updated_at = ?
		WHERE id = ?`, StatusProcessing, now, now, jobID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetJob(ctx, jobID)
}

func (r *Repository) nextAttemptNumber(ctx context.Context, jobID int64) (int, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt_no), 0) + 1 FROM email_attempt_logs WHERE job_id = ?`, jobID)
	var attemptNo int
	if err := row.Scan(&attemptNo); err != nil {
		return 0, err
	}
	return attemptNo, nil
}

func (r *Repository) CompleteAttempt(ctx context.Context, job *EmailJob, result *providerResult, sendErr error) error {
	attemptNo, err := r.nextAttemptNumber(ctx, job.ID)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	status := StatusSent
	providerMessageID := ""
	providerResponse := ""
	errorClass := ""
	errorMessage := ""
	if result != nil {
		providerMessageID = result.MessageID
		providerResponse = result.Response
	}
	if sendErr != nil {
		status = StatusFailed
		errorClass = "delivery_error"
		errorMessage = sendErr.Error()
	}

	logResult, err := tx.ExecContext(ctx, `INSERT INTO email_attempt_logs (
		job_id, attempt_no, status, provider_message_id, provider_response,
		error_class, error_message, subject, to_json, cc_json, bcc_json,
		started_at, finished_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, attemptNo, status, providerMessageID, providerResponse,
		errorClass, errorMessage, job.Subject, job.To, job.CC, job.BCC,
		now, now, now,
	)
	if err != nil {
		return err
	}
	attemptID, err := logResult.LastInsertId()
	if err != nil {
		return err
	}

	if status == StatusSent {
		_, err = tx.ExecContext(ctx, `UPDATE email_jobs
			SET status = ?, latest_attempt_id = ?, latest_error = '',
				sent_at = ?, failed_at = NULL, updated_at = ?
			WHERE id = ?`,
			StatusSent, attemptID, now, now, job.ID,
		)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE email_jobs
			SET status = ?, latest_attempt_id = ?, latest_error = ?,
				failed_at = ?, updated_at = ?
			WHERE id = ?`,
			StatusFailed, attemptID, errorMessage, now, now, job.ID,
		)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
