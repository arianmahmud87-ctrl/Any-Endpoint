package app

import (
	"context"
	"crypto/hmac"
	"errors"
	"net/http"
	"strings"
	"time"
)

type workerProvisioningJob struct {
	ID              string    `json:"job_id"`
	ProfileID       string    `json:"profile_id"`
	Provider        string    `json:"provider"`
	EnrollmentToken string    `json:"enrollment_token"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type workerProvisioningStatusRequest struct {
	State          string `json:"state"`
	FailureCode    string `json:"failure_code"`
	FailureMessage string `json:"failure_message"`
}

func provisioningTokenValid(cfg Config, raw string) bool {
	if !cfg.WorkerProvisioningEnabled || strings.TrimSpace(raw) == "" || cfg.WorkerProvisioningToken == "" {
		return false
	}
	return hmac.Equal([]byte(raw), []byte(cfg.WorkerProvisioningToken))
}

func (d *database) claimWorkerProvisioningJob(ctx context.Context, cfg Config) (workerProvisioningJob, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return workerProvisioningJob{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var job workerProvisioningJob
	var ciphertext []byte
	err = tx.QueryRow(ctx, `
		SELECT id::text, provider_profile_id::text, provider,
		       enrollment_token_ciphertext, expires_at
		FROM worker_provisioning_jobs
		WHERE expires_at > now()
		  AND (status = 'pending' OR (status = 'claimed' AND claimed_at < now() - interval '5 minutes'))
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1`).Scan(&job.ID, &job.ProfileID, &job.Provider, &ciphertext, &job.ExpiresAt)
	if err != nil {
		return workerProvisioningJob{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE worker_provisioning_jobs
		SET status = 'claimed', claimed_at = now(), updated_at = now()
		WHERE id = $1`, job.ID); err != nil {
		return workerProvisioningJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workerProvisioningJob{}, err
	}
	raw, err := decryptSecret(cfg.KeyPepper, ciphertext)
	if err != nil {
		return workerProvisioningJob{}, errors.New("worker provisioning secret could not be decrypted")
	}
	job.EnrollmentToken = raw
	return job, nil
}

func (d *database) failWorkerProvisioningJob(ctx context.Context, jobID, code, message string) error {
	result, err := d.pool.Exec(ctx, `
		UPDATE worker_provisioning_jobs
		SET status = 'failed', failure_code = NULLIF($2, ''), failure_message = NULLIF($3, ''),
		    completed_at = now(), updated_at = now()
		WHERE id = $1 AND status IN ('pending', 'claimed')`, jobID, truncate(strings.TrimSpace(code), 80), truncate(strings.TrimSpace(message), 240))
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("provisioning job not active")
	}
	return nil
}

func (s *server) internalWorkerProvisioning(w http.ResponseWriter, r *http.Request) {
	if !provisioningTokenValid(s.cfg, workerToken(r)) {
		writeError(w, http.StatusUnauthorized, "Provisioning authentication failed.", "authentication_error")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/internal/worker/provisioning/jobs" {
		job, err := s.db.claimWorkerProvisioningJob(r.Context(), s.cfg)
		if err != nil {
			if strings.Contains(err.Error(), "no rows") {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			writeError(w, http.StatusInternalServerError, "Provisioning queue is unavailable.", "api_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job": job})
		return
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/internal/worker/provisioning/jobs/") {
		jobID := strings.TrimPrefix(r.URL.Path, "/internal/worker/provisioning/jobs/")
		if jobID == "" || strings.Contains(jobID, "/") {
			writeError(w, http.StatusBadRequest, "Invalid provisioning job.", "invalid_request_error")
			return
		}
		var request workerProvisioningStatusRequest
		if err := decodeJSON(r, &request); err != nil || request.State != "failed" {
			writeError(w, http.StatusBadRequest, "Invalid provisioning status.", "invalid_request_error")
			return
		}
		if err := s.db.failWorkerProvisioningJob(r.Context(), jobID, request.FailureCode, request.FailureMessage); err != nil {
			writeError(w, http.StatusBadRequest, "Provisioning status was rejected.", "invalid_request_error")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}
	writeError(w, http.StatusNotFound, "Not found.", "invalid_request_error")
}
