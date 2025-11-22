package services

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/muety/wakapi/config"
	"github.com/muety/wakapi/models"
	"google.golang.org/api/option"

	"log/slog"
)

type BigQueryService struct {
	config *config.Config
	client *bigquery.Client
	table  *bigquery.Table
}

// BigQueryHeartbeat represents the schema for BigQuery heartbeat table
// It matches 100% with the PostgreSQL Heartbeat model
type BigQueryHeartbeat struct {
	ID               uint64    `bigquery:"id"`
	UserID           string    `bigquery:"user_id"`
	Entity           string    `bigquery:"entity"`
	Type             string    `bigquery:"type"`
	Category         string    `bigquery:"category"`
	Project          string    `bigquery:"project"`
	Branch           string    `bigquery:"branch"`
	Language         string    `bigquery:"language"`
	IsWrite          bool      `bigquery:"is_write"`
	Editor           string    `bigquery:"editor"`
	OperatingSystem  string    `bigquery:"operating_system"`
	Machine          string    `bigquery:"machine"`
	UserAgent        string    `bigquery:"user_agent"`
	Time             time.Time `bigquery:"time"`
	Hash             string    `bigquery:"hash"`
	Origin           string    `bigquery:"origin"`
	OriginId         string    `bigquery:"origin_id"`
	CreatedAt        time.Time `bigquery:"created_at"`
	Lines            int       `bigquery:"lines"`
	LineNo           int       `bigquery:"lineno"`
	CursorPos        int       `bigquery:"cursorpos"`
	LineDeletions    int       `bigquery:"line_deletions"`
	LineAdditions    int       `bigquery:"line_additions"`
	ProjectRootCount int       `bigquery:"project_root_count"`
}

func NewBigQueryService() (*BigQueryService, error) {
	cfg := config.Get()

	if !cfg.BigQuery.Enabled {
		return nil, nil
	}

	ctx := context.Background()

	// Create BigQuery client with service account credentials
	client, err := bigquery.NewClient(
		ctx,
		cfg.BigQuery.ProjectID,
		option.WithCredentialsFile(cfg.BigQuery.ServiceAccountJsonPath),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create bigquery client: %w", err)
	}

	// Get reference to the table
	table := client.Dataset(cfg.BigQuery.DatasetID).Table(cfg.BigQuery.TableID)

	// Check if table exists, if not, create it
	if _, err := table.Metadata(ctx); err != nil {
		slog.Info("bigquery table does not exist, creating it",
			"dataset", cfg.BigQuery.DatasetID,
			"table", cfg.BigQuery.TableID)

		schema, err := bigquery.InferSchema(BigQueryHeartbeat{})
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to infer bigquery schema: %w", err)
		}

		if err := table.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to create bigquery table: %w", err)
		}

		slog.Info("bigquery table created successfully")
	}

	slog.Info("bigquery service initialized successfully")

	return &BigQueryService{
		config: cfg,
		client: client,
		table:  table,
	}, nil
}

// InsertHeartbeats writes a batch of heartbeats to BigQuery
func (s *BigQueryService) InsertHeartbeats(heartbeats []*models.Heartbeat) error {
	if s == nil || s.client == nil {
		return nil // BigQuery is disabled
	}

	if len(heartbeats) == 0 {
		return nil
	}

	ctx := context.Background()
	inserter := s.table.Inserter()

	// Convert model heartbeats to BigQuery format
	bqHeartbeats := make([]*BigQueryHeartbeat, 0, len(heartbeats))
	for _, hb := range heartbeats {
		bqHeartbeats = append(bqHeartbeats, &BigQueryHeartbeat{
			ID:               hb.ID,
			UserID:           hb.UserID,
			Entity:           hb.Entity,
			Type:             hb.Type,
			Category:         hb.Category,
			Project:          hb.Project,
			Branch:           hb.Branch,
			Language:         hb.Language,
			IsWrite:          hb.IsWrite,
			Editor:           hb.Editor,
			OperatingSystem:  hb.OperatingSystem,
			Machine:          hb.Machine,
			UserAgent:        hb.UserAgent,
			Time:             hb.Time.T(),
			Hash:             hb.Hash,
			Origin:           hb.Origin,
			OriginId:         hb.OriginId,
			CreatedAt:        hb.CreatedAt.T(),
			Lines:            hb.Lines,
			LineNo:           hb.LineNo,
			CursorPos:        hb.CursorPos,
			LineDeletions:    hb.LineDeletions,
			LineAdditions:    hb.LineAdditions,
			ProjectRootCount: hb.ProjectRootCount,
		})
	}

	// Insert into BigQuery
	if err := inserter.Put(ctx, bqHeartbeats); err != nil {
		return fmt.Errorf("failed to insert heartbeats to bigquery: %w", err)
	}

	slog.Debug("inserted heartbeats to bigquery", "count", len(bqHeartbeats))

	return nil
}

// Close closes the BigQuery client
func (s *BigQueryService) Close() error {
	if s != nil && s.client != nil {
		return s.client.Close()
	}
	return nil
}
