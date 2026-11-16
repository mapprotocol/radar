package xrp

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/storage"
)

type latestSaver struct {
	latest []uint64
}

func (s *latestSaver) Type() string                         { return "fake" }
func (s *latestSaver) Mos(uint64, []*dao.Mos) error         { return nil }
func (s *latestSaver) ScanBlockNumber(string, uint64) error { return nil }
func (s *latestSaver) GetEvent(int64) ([]*dao.Event, error) { return nil, nil }
func (s *latestSaver) LatestBlockNumber(_ string, n uint64) error {
	s.latest = append(s.latest, n)
	return nil
}

func newLatestChain(s storage.Saver, isBackUp bool) *Chain {
	return &Chain{
		log:      log.New(),
		cfg:      &Config{},
		storages: []storage.Saver{s},
		isBackUp: isBackUp,
	}
}

func TestSaveLatestBlockThrottled(t *testing.T) {
	s := &latestSaver{}
	c := newLatestChain(s, false)
	now := time.Unix(1000, 0)

	c.saveLatestBlock(100, now)
	c.saveLatestBlock(101, now.Add(latestSaveInterval-time.Second))
	c.saveLatestBlock(102, now.Add(latestSaveInterval))
	c.saveLatestBlock(102, now.Add(3*latestSaveInterval))

	if len(s.latest) != 2 || s.latest[0] != 100 || s.latest[1] != 102 {
		t.Fatalf("latest = %v, want [100 102]", s.latest)
	}
}

func TestSaveLatestBlockSkippedOnBackUp(t *testing.T) {
	s := &latestSaver{}
	c := newLatestChain(s, true)

	c.saveLatestBlock(100, time.Unix(1000, 0))

	if len(s.latest) != 0 {
		t.Fatalf("backup instance saved latest %v, want nothing", s.latest)
	}
}
