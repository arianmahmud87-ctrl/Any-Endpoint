package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type database struct {
	pool  *pgxpool.Pool
	redis *redis.Client
}

func openDatabase(ctx context.Context, cfg Config) (*database, error) {
	if !cfg.AuthEnabled {
		return nil, nil
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	client := redis.NewClient(redisOptions)
	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		pool.Close()
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	return &database{pool: pool, redis: client}, nil
}

func (d *database) close() {
	if d == nil {
		return
	}
	if d.redis != nil {
		_ = d.redis.Close()
	}
	if d.pool != nil {
		d.pool.Close()
	}
}

func (d *database) ping(ctx context.Context) error {
	if d == nil {
		return errors.New("database is not configured")
	}
	if err := d.pool.Ping(ctx); err != nil {
		return err
	}
	return d.redis.Ping(ctx).Err()
}

type identity struct {
	UserID         string
	OrganizationID string
	Role           string
	Email          string
	EmailVerified  bool
}

func (d *database) upsertIdentity(ctx context.Context, subject, email string, emailVerified bool) (identity, error) {
	if d == nil {
		return identity{}, errors.New("database is not configured")
	}
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return identity{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (google_subject, email, email_verified)
		VALUES ($1, $2, $3)
		ON CONFLICT (google_subject) DO UPDATE SET
			email = EXCLUDED.email,
			email_verified = EXCLUDED.email_verified,
			updated_at = now()
		RETURNING id::text`, subject, email, emailVerified).Scan(&userID); err != nil {
		return identity{}, fmt.Errorf("upsert user: %w", err)
	}

	var organizationID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO organizations (name, owner_user_id, personal)
		VALUES ('Personal workspace', $1, true)
		ON CONFLICT (owner_user_id) WHERE personal DO UPDATE SET name = organizations.name
		RETURNING id::text`, userID).Scan(&organizationID); err != nil {
		return identity{}, fmt.Errorf("bootstrap organization: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO organization_memberships (organization_id, user_id, role)
		VALUES ($1, $2, 'owner')
		ON CONFLICT (organization_id, user_id) DO UPDATE SET role = 'owner'`, organizationID, userID); err != nil {
		return identity{}, fmt.Errorf("bootstrap membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return identity{}, fmt.Errorf("commit identity bootstrap: %w", err)
	}
	return identity{UserID: userID, OrganizationID: organizationID, Role: "owner", Email: email, EmailVerified: emailVerified}, nil
}

type sessionRecord struct {
	identity
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (d *database) createSession(ctx context.Context, cfg Config, id identity, userAgent, ipHash string) (string, error) {
	raw, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	digest := keyedDigest(cfg.SessionSecret, raw)
	expires := time.Now().Add(cfg.SessionTTL)
	_, err = d.pool.Exec(ctx, `
		INSERT INTO sessions (session_digest, user_id, organization_id, user_agent, ip_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, digest, id.UserID, id.OrganizationID, truncate(userAgent, 512), truncate(ipHash, 128), expires)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (d *database) loadSession(ctx context.Context, cfg Config, raw string) (sessionRecord, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return sessionRecord{}, errors.New("invalid session")
	}
	digest := keyedDigest(cfg.SessionSecret, decoded)
	var result sessionRecord
	err = d.pool.QueryRow(ctx, `
		SELECT s.created_at, s.expires_at, u.id::text, u.email, u.email_verified,
		       s.organization_id::text, m.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN organization_memberships m
		  ON m.organization_id = s.organization_id AND m.user_id = s.user_id
		WHERE s.session_digest = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()`, digest).Scan(
		&result.CreatedAt, &result.ExpiresAt, &result.UserID, &result.Email,
		&result.EmailVerified, &result.OrganizationID, &result.Role)
	if err != nil {
		return sessionRecord{}, errors.New("invalid session")
	}
	_, _ = d.pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE session_digest = $1`, digest)
	return result, nil
}

func (d *database) revokeSession(ctx context.Context, cfg Config, raw string) error {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return nil
	}
	_, err = d.pool.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE session_digest = $1`, keyedDigest(cfg.SessionSecret, decoded))
	return err
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	_, err := rand.Read(value)
	return value, err
}

func keyedDigest(secret string, value []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(value)
	return mac.Sum(nil)
}

func csrfToken(secret, rawSession string) string {
	return base64.RawURLEncoding.EncodeToString(keyedDigest(secret, []byte("csrf:"+rawSession)))
}

func hashAddress(secret string, r *http.Request) string {
	address := r.RemoteAddr
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	return base64.RawURLEncoding.EncodeToString(keyedDigest(secret, []byte(address)))
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func encryptSecret(key, plaintext string) ([]byte, error) {
	keySum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(keySum[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func decryptSecret(key string, ciphertext []byte) (string, error) {
	keySum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(keySum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted secret")
	}
	nonce, payload := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", errors.New("encrypted secret authentication failed")
	}
	return string(plaintext), nil
}
