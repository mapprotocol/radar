package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type EventStatistics struct {
	db *gorm.DB
}

type EventCount struct {
	ChainId   int64
	ProjectId int64
	Count     int64
}

func NewEventStatistics(db *gorm.DB) *EventStatistics {
	return &EventStatistics{db: db}
}

func (s *EventStatistics) Chains(ctx context.Context) ([]string, error) {
	var chains []string
	err := s.db.WithContext(ctx).Raw(`
		SELECT chain_id FROM block WHERE chain_id IS NOT NULL AND chain_id <> ''
		UNION SELECT chain_id FROM scan_block WHERE chain_id IS NOT NULL AND chain_id <> ''
		UNION SELECT chain_id FROM event WHERE chain_id IS NOT NULL AND chain_id <> ''
	`).Scan(&chains).Error
	return chains, err
}

func (s *EventStatistics) Count(ctx context.Context, start, end time.Time) ([]EventCount, error) {
	var counts []EventCount
	// Aggregate the indexed time range before joining definitions to avoid rescanning
	// a project's complete history for every matching event definition.
	occurrences := s.db.WithContext(ctx).Table("mos").
		Select("chain_id, project_id, event_id, COUNT(*) AS event_count").
		Where("project_id IN ?", []int64{1, 2}).
		Where("tx_timestamp >= ? AND tx_timestamp < ?", start.Unix(), end.Unix()).
		Group("chain_id, project_id, event_id")
	// Match the event definition by ID and project: mos.topic also contains indexed arguments.
	err := s.db.WithContext(ctx).Table("(?) AS m", occurrences).
		Select("m.chain_id, m.project_id, SUM(m.event_count) AS count").
		Joins("JOIN event AS e ON e.id = m.event_id AND e.project_id = m.project_id").
		Where("(m.project_id = ? AND CAST(e.format AS BINARY) LIKE ?) OR (m.project_id = ? AND CAST(e.format AS BINARY) LIKE ?)",
			1, "MessageOut(%", 2, "MessageIn(%").
		Group("m.chain_id, m.project_id").Scan(&counts).Error
	return counts, err
}
