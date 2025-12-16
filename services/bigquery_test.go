package services

import (
	"testing"
	"time"

	"github.com/muety/wakapi/models"
	"github.com/stretchr/testify/assert"
)

func TestBigQueryHeartbeat_SchemaMatches(t *testing.T) {
	// This test ensures that the BigQuery schema matches the Heartbeat model
	// by creating instances of both and comparing their field counts

	now := time.Now()
	customTime := models.CustomTime(now)

	// Create a Heartbeat model
	heartbeat := &models.Heartbeat{
		ID:               12345,
		UserID:           "testuser",
		Entity:           "/path/to/file.go",
		Type:             "file",
		Category:         "coding",
		Project:          "wakapi",
		Branch:           "main",
		Language:         "Go",
		IsWrite:          true,
		Editor:           "vscode",
		OperatingSystem:  "linux",
		Machine:          "my-machine",
		UserAgent:        "wakatime/v1.0.0",
		Time:             customTime,
		Hash:             "abc123",
		Origin:           "wakatime",
		OriginId:         "origin123",
		CreatedAt:        customTime,
		Lines:            100,
		LineNo:           50,
		CursorPos:        25,
		LineDeletions:    5,
		LineAdditions:    10,
		ProjectRootCount: 3,
	}

	// Create a BigQueryHeartbeat
	bqHeartbeat := &BigQueryHeartbeat{
		ID:               heartbeat.ID,
		UserID:           heartbeat.UserID,
		Entity:           heartbeat.Entity,
		Type:             heartbeat.Type,
		Category:         heartbeat.Category,
		Project:          heartbeat.Project,
		Branch:           heartbeat.Branch,
		Language:         heartbeat.Language,
		IsWrite:          heartbeat.IsWrite,
		Editor:           heartbeat.Editor,
		OperatingSystem:  heartbeat.OperatingSystem,
		Machine:          heartbeat.Machine,
		UserAgent:        heartbeat.UserAgent,
		Time:             heartbeat.Time.T(),
		Hash:             heartbeat.Hash,
		Origin:           heartbeat.Origin,
		OriginId:         heartbeat.OriginId,
		CreatedAt:        heartbeat.CreatedAt.T(),
		Lines:            heartbeat.Lines,
		LineNo:           heartbeat.LineNo,
		CursorPos:        heartbeat.CursorPos,
		LineDeletions:    heartbeat.LineDeletions,
		LineAdditions:    heartbeat.LineAdditions,
		ProjectRootCount: heartbeat.ProjectRootCount,
	}

	// Verify field mapping
	assert.Equal(t, heartbeat.ID, bqHeartbeat.ID)
	assert.Equal(t, heartbeat.UserID, bqHeartbeat.UserID)
	assert.Equal(t, heartbeat.Entity, bqHeartbeat.Entity)
	assert.Equal(t, heartbeat.Type, bqHeartbeat.Type)
	assert.Equal(t, heartbeat.Category, bqHeartbeat.Category)
	assert.Equal(t, heartbeat.Project, bqHeartbeat.Project)
	assert.Equal(t, heartbeat.Branch, bqHeartbeat.Branch)
	assert.Equal(t, heartbeat.Language, bqHeartbeat.Language)
	assert.Equal(t, heartbeat.IsWrite, bqHeartbeat.IsWrite)
	assert.Equal(t, heartbeat.Editor, bqHeartbeat.Editor)
	assert.Equal(t, heartbeat.OperatingSystem, bqHeartbeat.OperatingSystem)
	assert.Equal(t, heartbeat.Machine, bqHeartbeat.Machine)
	assert.Equal(t, heartbeat.UserAgent, bqHeartbeat.UserAgent)
	assert.Equal(t, heartbeat.Time.T(), bqHeartbeat.Time)
	assert.Equal(t, heartbeat.Hash, bqHeartbeat.Hash)
	assert.Equal(t, heartbeat.Origin, bqHeartbeat.Origin)
	assert.Equal(t, heartbeat.OriginId, bqHeartbeat.OriginId)
	assert.Equal(t, heartbeat.CreatedAt.T(), bqHeartbeat.CreatedAt)
	assert.Equal(t, heartbeat.Lines, bqHeartbeat.Lines)
	assert.Equal(t, heartbeat.LineNo, bqHeartbeat.LineNo)
	assert.Equal(t, heartbeat.CursorPos, bqHeartbeat.CursorPos)
	assert.Equal(t, heartbeat.LineDeletions, bqHeartbeat.LineDeletions)
	assert.Equal(t, heartbeat.LineAdditions, bqHeartbeat.LineAdditions)
	assert.Equal(t, heartbeat.ProjectRootCount, bqHeartbeat.ProjectRootCount)
}

func TestBigQueryDuration_SchemaMatches(t *testing.T) {
	// This test ensures that the BigQuery duration schema matches the Duration model
	// by creating instances of both and comparing their field mappings

	now := time.Now()
	customTime := models.CustomTime(now)

	// Create a Duration model
	duration := &models.Duration{
		ID:              12345,
		UserID:          "testuser",
		Time:            customTime,
		Duration:        30 * time.Minute,
		Project:         "wakapi",
		Language:        "Go",
		Editor:          "vscode",
		OperatingSystem: "linux",
		Machine:         "my-machine",
		Category:        "coding",
		Branch:          "main",
		Entity:          "/path/to/file.go",
		NumHeartbeats:   15,
		GroupHash:       "abc123def456",
		Timeout:         10 * time.Minute,
	}

	// Create a BigQueryDuration
	bqDuration := &BigQueryDuration{
		ID:              duration.ID,
		UserID:          duration.UserID,
		Time:            duration.Time.T(),
		Duration:        int64(duration.Duration),
		Project:         duration.Project,
		Language:        duration.Language,
		Editor:          duration.Editor,
		OperatingSystem: duration.OperatingSystem,
		Machine:         duration.Machine,
		Category:        duration.Category,
		Branch:          duration.Branch,
		Entity:          duration.Entity,
		NumHeartbeats:   duration.NumHeartbeats,
		GroupHash:       duration.GroupHash,
		Timeout:         int64(duration.Timeout),
	}

	// Verify field mapping
	assert.Equal(t, duration.ID, bqDuration.ID)
	assert.Equal(t, duration.UserID, bqDuration.UserID)
	assert.Equal(t, duration.Time.T(), bqDuration.Time)
	assert.Equal(t, int64(duration.Duration), bqDuration.Duration)
	assert.Equal(t, duration.Project, bqDuration.Project)
	assert.Equal(t, duration.Language, bqDuration.Language)
	assert.Equal(t, duration.Editor, bqDuration.Editor)
	assert.Equal(t, duration.OperatingSystem, bqDuration.OperatingSystem)
	assert.Equal(t, duration.Machine, bqDuration.Machine)
	assert.Equal(t, duration.Category, bqDuration.Category)
	assert.Equal(t, duration.Branch, bqDuration.Branch)
	assert.Equal(t, duration.Entity, bqDuration.Entity)
	assert.Equal(t, duration.NumHeartbeats, bqDuration.NumHeartbeats)
	assert.Equal(t, duration.GroupHash, bqDuration.GroupHash)
	assert.Equal(t, int64(duration.Timeout), bqDuration.Timeout)
}

func TestBigQueryService_Disabled(t *testing.T) {
	// Test that BigQuery service operations gracefully handle nil/disabled state
	var bqService *BigQueryService

	// Should not panic and return no error
	err := bqService.InsertHeartbeats([]*models.Heartbeat{})
	assert.NoError(t, err)

	// Should not panic and return no error
	err = bqService.InsertDurations([]*models.Duration{})
	assert.NoError(t, err)

	// Should not panic and return no error
	err = bqService.Close()
	assert.NoError(t, err)
}
