package logger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aiNrve/proxy/internal/models"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

const (
	defaultBufferSize = 1000
	defaultBatchSize  = 100
	defaultFlushEvery = 1 * time.Second
)

const createSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS request_logs (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id        TEXT NOT NULL,
  provider          TEXT NOT NULL,
  model             TEXT NOT NULL,
  task_type         TEXT,
  prompt_tokens     INT,
  completion_tokens INT,
  cost_usd          NUMERIC(10,8),
  latency_ms        INT,
  error             TEXT,
  created_at        TIMESTAMPTZ DEFAULT NOW()
);
`

const insertLogSQL = `
INSERT INTO request_logs (
  request_id,
  provider,
  model,
  task_type,
  prompt_tokens,
  completion_tokens,
  cost_usd,
  latency_ms,
  error,
  created_at
) VALUES (
  :request_id,
  :provider,
  :model,
  :task_type,
  :prompt_tokens,
  :completion_tokens,
  :cost_usd,
  :latency_ms,
  :error,
  :created_at
)
`

// Logger asynchronously persists request logs.
type Logger struct {
	log        *zap.Logger
	db         *sqlx.DB
	stdoutOnly bool

	ch            chan *models.RequestLog
	batchSize     int
	flushInterval time.Duration

	stopCh    chan struct{}
	doneCh    chan struct{}
	closed    atomic.Bool
	closeOnce sync.Once
}

// New creates and starts an async logger worker.
func New(databaseURL string, baseLogger *zap.Logger) (*Logger, error) {
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}

	l := &Logger{
		log:           baseLogger,
		stdoutOnly:    databaseURL == "",
		ch:            make(chan *models.RequestLog, defaultBufferSize),
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushEvery,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}

	if !l.stdoutOnly {
		db, err := sqlx.Open("postgres", databaseURL)
		if err != nil {
			return nil, fmt.Errorf("open postgres connection: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}

		if _, err := db.ExecContext(ctx, createSchemaSQL); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("ensure logging schema: %w", err)
		}

		l.db = db
	}

	go l.runWorker()
	return l, nil
}

// Enqueue adds a request log without blocking. It returns false when dropped.
func (l *Logger) Enqueue(entry *models.RequestLog) bool {
	if entry == nil {
		return false
	}
	if l.closed.Load() {
		return false
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	select {
	case l.ch <- entry:
		return true
	default:
		l.log.Warn("dropping request log because queue is full", zap.String("request_id", entry.RequestID))
		return false
	}
}

// Close flushes buffered logs and stops the worker.
func (l *Logger) Close(ctx context.Context) error {
	var closeErr error
	l.closeOnce.Do(func() {
		l.closed.Store(true)
		close(l.stopCh)

		select {
		case <-l.doneCh:
		case <-ctx.Done():
			closeErr = ctx.Err()
		}

		if l.db != nil {
			if err := l.db.Close(); err != nil {
				closeErr = errors.Join(closeErr, err)
			}
		}
	})
	return closeErr
}

func (l *Logger) runWorker() {
	defer close(l.doneCh)

	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	batch := make([]*models.RequestLog, 0, l.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.flushBatch(batch); err != nil {
			l.log.Error("failed to flush request log batch", zap.Error(err), zap.Int("batch_size", len(batch)))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-l.stopCh:
			for {
				select {
				case item := <-l.ch:
					if item != nil {
						batch = append(batch, item)
					}
				default:
					flush()
					return
				}
			}
		case item := <-l.ch:
			if item == nil {
				continue
			}
			batch = append(batch, item)
			if len(batch) >= l.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (l *Logger) flushBatch(batch []*models.RequestLog) error {
	if l.stdoutOnly {
		for _, entry := range batch {
			payload, err := json.MarshalIndent(entry, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal log entry: %w", err)
			}
			l.log.Info("request_log", zap.ByteString("entry", payload))
		}
		return nil
	}

	if l.db == nil {
		return errors.New("postgres mode enabled without database connection")
	}

	tx, err := l.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	rows := make([]models.RequestLog, 0, len(batch))
	for _, item := range batch {
		if item == nil {
			continue
		}
		rows = append(rows, *item)
	}

	if len(rows) == 0 {
		return nil
	}

	if _, err := tx.NamedExec(insertLogSQL, rows); err != nil {
		return fmt.Errorf("batch insert request logs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		if !errors.Is(err, sql.ErrTxDone) {
			return fmt.Errorf("commit request logs: %w", err)
		}
	}
	return nil
}
