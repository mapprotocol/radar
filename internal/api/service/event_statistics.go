package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/filter/internal/api/store/mysql"
	"github.com/mapprotocol/filter/internal/api/stream"
)

const eventStatisticsDays = 7

var ErrEventStatisticsUnavailable = errors.New("event statistics cache is not ready; retry later")

type dailyEventCounts struct {
	counts    []mysql.EventCount
	updatedAt int64
	final     bool
}

type EventStatistics struct {
	store     *mysql.EventStatistics
	location  *time.Location
	mu        sync.RWMutex
	refresh   sync.Mutex
	days      map[string]dailyEventCounts
	chains    []int64
	updatedAt int64
}

func NewEventStatistics(store *mysql.EventStatistics, timezone string) (*EventStatistics, error) {
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid event_statistics_timezone: %w", err)
	}
	return &EventStatistics{
		store: store, location: location, days: make(map[string]dailyEventCounts),
	}, nil
}

// Start warms the cache immediately, then refreshes at each hour. The returned stop waits for shutdown.
func (s *EventStatistics) Start(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			now := time.Now().In(s.location)
			next := now.Add(time.Hour - time.Duration(now.Minute())*time.Minute -
				time.Duration(now.Second())*time.Second - time.Duration(now.Nanosecond()))
			refreshCtx, refreshCancel := context.WithTimeout(ctx, 5*time.Minute)
			err := s.Refresh(refreshCtx, now)
			refreshCancel()
			if err != nil && ctx.Err() == nil {
				log.Error("refresh event statistics failed", "err", err)
			}
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func (s *EventStatistics) dayDate(now time.Time) time.Time {
	now = now.In(s.location)
	// Iterate civil dates in UTC: some timezones skip midnight during DST changes.
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *EventStatistics) midnight(date time.Time) time.Time {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, s.location)
	if start.Format(time.DateOnly) < date.Format(time.DateOnly) {
		// A nonexistent midnight can normalize backwards; the transition starts the day.
		_, end := start.ZoneBounds()
		if !end.IsZero() {
			return end
		}
	}
	return start
}

func (s *EventStatistics) Refresh(ctx context.Context, now time.Time) error {
	s.refresh.Lock()
	defer s.refresh.Unlock()

	today := s.dayDate(now)
	first := today.AddDate(0, 0, 1-eventStatisticsDays)
	chains, err := s.store.Chains(ctx)
	if err != nil {
		return fmt.Errorf("list statistics chains: %w", err)
	}
	chainSet := make(map[int64]struct{}, len(chains))
	for _, chain := range chains {
		id, err := strconv.ParseInt(chain, 10, 64)
		if err == nil && id > 0 {
			chainSet[id] = struct{}{}
		}
	}

	nextDays := make(map[string]dailyEventCounts, eventStatisticsDays)
	for day := first; !day.After(today); day = day.AddDate(0, 0, 1) {
		date := day.Format(time.DateOnly)
		s.mu.RLock()
		cached, ok := s.days[date]
		s.mu.RUnlock()
		// Allow one hourly interval after midnight for delayed confirmations to arrive.
		if !ok || !cached.final {
			end := s.midnight(day.AddDate(0, 0, 1))
			final := !end.Add(time.Hour).After(now)
			if end.After(now) {
				end = now.Truncate(time.Second).Add(time.Second)
			}
			counts, err := s.store.Count(ctx, s.midnight(day), end)
			if err != nil {
				return fmt.Errorf("count events for %s: %w", date, err)
			}
			cached = dailyEventCounts{counts: counts, updatedAt: now.Unix(), final: final}
		}
		nextDays[date] = cached
		for _, count := range cached.counts {
			if count.ChainId > 0 {
				chainSet[count.ChainId] = struct{}{}
			}
		}
	}

	nextChains := make([]int64, 0, len(chainSet))
	for chain := range chainSet {
		nextChains = append(nextChains, chain)
	}
	sort.Slice(nextChains, func(i, j int) bool { return nextChains[i] < nextChains[j] })
	s.mu.Lock()
	s.days, s.chains, s.updatedAt = nextDays, nextChains, now.Unix()
	s.mu.Unlock()
	return nil
}

func (s *EventStatistics) Get(now time.Time) (*stream.EventStatisticsResp, error) {
	today := s.dayDate(now)
	first := today.AddDate(0, 0, 1-eventStatisticsDays)
	s.mu.RLock()
	defer s.mu.RUnlock()

	ret := &stream.EventStatisticsResp{
		Timezone: s.location.String(), StartDate: first.Format(time.DateOnly),
		EndDate: today.Format(time.DateOnly), UpdatedAt: s.updatedAt,
		Days: make([]stream.DailyEventStatistics, 0, eventStatisticsDays),
	}
	for day := first; !day.After(today); day = day.AddDate(0, 0, 1) {
		date := day.Format(time.DateOnly)
		cached, ok := s.days[date]
		if !ok {
			return nil, ErrEventStatisticsUnavailable
		}
		counts := make(map[int64]stream.ChainEventStatistics, len(s.chains))
		for _, count := range cached.counts {
			value := counts[count.ChainId]
			if count.ProjectId == 1 {
				value.MessageOut = count.Count
			} else if count.ProjectId == 2 {
				value.MessageIn = count.Count
			}
			counts[count.ChainId] = value
		}
		daily := stream.DailyEventStatistics{
			Date: date, UpdatedAt: cached.updatedAt,
			Chains: make([]stream.ChainEventStatistics, 0, len(s.chains)),
		}
		for _, chain := range s.chains {
			value := counts[chain]
			value.ChainId = strconv.FormatInt(chain, 10)
			daily.Chains = append(daily.Chains, value)
		}
		ret.Days = append(ret.Days, daily)
	}
	return ret, nil
}
