package ethereum

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/mapprotocol/filter/internal/pkg/dao"
	"github.com/mapprotocol/filter/internal/pkg/storage"
)

type scanSaver struct {
	saved []uint64
}

func (s *scanSaver) Type() string                           { return "fake" }
func (s *scanSaver) Mos(uint64, []*dao.Mos) error           { return nil }
func (s *scanSaver) LatestBlockNumber(string, uint64) error { return nil }
func (s *scanSaver) GetEvent(int64) ([]*dao.Event, error)   { return nil, nil }
func (s *scanSaver) ScanBlockNumber(_ string, n uint64) error {
	s.saved = append(s.saved, n)
	return nil
}

func newScanChain(s storage.Saver, isBackUp bool) *Chain {
	return &Chain{
		log:      log.New(),
		cfg:      &EthConfig{},
		storages: []storage.Saver{s},
		isBackUp: isBackUp,
	}
}

func TestSaveScanProgressThrottled(t *testing.T) {
	s := &scanSaver{}
	c := newScanChain(s, false)
	now := time.Unix(1000, 0)

	c.saveScanProgress(100, now)
	c.saveScanProgress(200, now.Add(scanSaveInterval-time.Second))
	c.saveScanProgress(300, now.Add(scanSaveInterval))

	if len(s.saved) != 2 || s.saved[0] != 100 || s.saved[1] != 300 {
		t.Fatalf("saved = %v, want [100 300]", s.saved)
	}
}

func TestSaveScanProgressSkippedOnBackUp(t *testing.T) {
	s := &scanSaver{}
	c := newScanChain(s, true)

	c.saveScanProgress(100, time.Unix(1000, 0))

	if len(s.saved) != 0 {
		t.Fatalf("backup instance saved %v, want nothing", s.saved)
	}
}
