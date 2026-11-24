package solana

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pkg/errors"

	"github.com/mapprotocol/filter/pkg/utils"
)

const (
	signaturePageLimit = 100
	roundInterval      = 3 * time.Second
	handleRetryDelay   = time.Second
	handleAttempts     = 3
	rpcTimeout         = time.Minute
	latestSaveInterval = 3 * time.Second
)

var (
	getTransactionRetryDelays = []time.Duration{10 * time.Second, 30 * time.Second, 60 * time.Second}
	maxTxVersion              = uint64(0)
	retryableErr              = regexp.MustCompile(`(?i)(^|[^0-9])(429|500|502|503|504)([^0-9]|$)|timeout|timed out|fetch failed|ECONNRESET|ETIMEDOUT|connection reset|connection refused|EOF|INVALID_BUTTER_RESPONSE`)
)

func (c *Chain) sync() error {
	for {
		select {
		case <-c.stop:
			c.log.Info("Stop syncing solana signatures")
			return nil
		default:
		}
		if err := c.getMatch(); err != nil {
			c.log.Error("Renew events failed", "err", err)
		}
		c.syncOnce()
		c.sleep(roundInterval)
	}
}

// syncOnce walks every program once like compass-ts SolChain.syncOnce; a tx shared by programs is handled once.
func (c *Chain) syncOnce() {
	handled := make(map[string]struct{})
	for _, program := range c.cfg.Mcs {
		begin := c.cursors[program]
		next, progress, err := c.syncProgram(program, begin, handled)
		if err != nil {
			c.state.RecordError("solana_sync", err.Error())
			c.log.Error("Solana sync program failed", "program", program, "begin", begin, "err", err)
			continue
		}
		c.cursors[program] = next
		c.saveCursor(program, next)
		if progress > c.progress[program] {
			c.progress[program] = progress
		}
	}
	c.saveScanProgress()
}

// syncProgram handles new signatures oldest first and returns the new cursor plus the slot up to which
// every signature of the program is handled. A failing tx stops the walk so it is retried next round.
func (c *Chain) syncProgram(program solana.PublicKey, begin string, handled map[string]struct{}) (string, uint64, error) {
	finalized, err := c.finalizedSlot()
	if err != nil {
		return begin, 0, err
	}
	sigs, err := c.signaturesSince(program, begin)
	if err != nil {
		return begin, 0, err
	}
	if len(sigs) == 0 {
		c.sleep(roundInterval)
		return begin, finalized, nil
	}

	next := begin
	for i := len(sigs) - 1; i >= 0; i-- {
		sig := sigs[i]
		txHash := sig.Signature.String()
		if _, ok := handled[txHash]; ok {
			c.log.Info("Skip duplicate solana tx", "hash", txHash)
			next = txHash
			continue
		}
		c.log.Info("Handle solana tx", "program", program, "hash", txHash, "slot", sig.Slot)

		tx, err := c.getTransactionWithRetry(sig.Signature)
		if err != nil {
			c.alertFailure(program, txHash, "getTransaction", err)
			return next, sig.Slot - 1, nil
		}
		if err = c.handleTransactionWithRetry(txHash, tx); err != nil {
			c.alertFailure(program, txHash, "handleTransaction", err)
			return next, sig.Slot - 1, nil
		}
		handled[txHash] = struct{}{}
		next = txHash
	}
	if sigs[0].Slot > finalized {
		finalized = sigs[0].Slot
	}
	return next, finalized, nil
}

func (c *Chain) finalizedSlot() (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()
	start := time.Now()
	slot, err := c.rpc.GetSlot(ctx, rpc.CommitmentFinalized)
	c.state.ObserveRPC("getSlot", time.Since(start).Seconds())
	if err != nil {
		c.state.RecordError("rpc_latest_block", err.Error())
		return 0, errors.Wrap(err, "get finalized slot")
	}
	c.state.SetLatestBlock(int64(slot))
	c.saveLatestBlock(slot, time.Now())
	return slot, nil
}

// signaturesSince pages getSignaturesForAddress newest first until the cursor signature.
func (c *Chain) signaturesSince(program solana.PublicKey, begin string) ([]*rpc.TransactionSignature, error) {
	var until solana.Signature
	if begin != "" {
		s, err := solana.SignatureFromBase58(begin)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid cursor %s", begin)
		}
		until = s
	}
	limit := signaturePageLimit
	var (
		ret    []*rpc.TransactionSignature
		before solana.Signature
	)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		start := time.Now()
		page, err := c.rpc.GetSignaturesForAddressWithOpts(ctx, program, &rpc.GetSignaturesForAddressOpts{
			Limit:      &limit,
			Before:     before,
			Until:      until,
			Commitment: rpc.CommitmentFinalized,
		})
		cancel()
		c.state.ObserveRPC("getSignaturesForAddress", time.Since(start).Seconds())
		if err != nil {
			return nil, errors.Wrap(err, "get signatures for address")
		}
		if len(page) == 0 {
			break
		}
		ret = append(ret, page...)
		if len(page) < signaturePageLimit {
			break
		}
		nextBefore := page[len(page)-1].Signature
		if nextBefore == before {
			break
		}
		before = nextBefore
	}
	return ret, nil
}

func (c *Chain) getTransactionWithRetry(sig solana.Signature) (*rpc.GetTransactionResult, error) {
	var lastErr error
	for attempt := 0; attempt <= len(getTransactionRetryDelays); attempt++ {
		if attempt > 0 {
			c.log.Warn("Solana retry getTransaction", "hash", sig, "attempt", attempt, "err", lastErr)
			c.sleep(getTransactionRetryDelays[attempt-1])
		}
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		start := time.Now()
		tx, err := c.rpc.GetTransaction(ctx, sig, &rpc.GetTransactionOpts{
			Commitment:                     rpc.CommitmentFinalized,
			MaxSupportedTransactionVersion: &maxTxVersion,
		})
		cancel()
		c.state.ObserveRPC("getTransaction", time.Since(start).Seconds())
		if err == nil && tx == nil {
			err = errors.New("solana getTransaction returned null")
		}
		if err == nil {
			return tx, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c *Chain) handleTransactionWithRetry(txHash string, tx *rpc.GetTransactionResult) error {
	var err error
	for attempt := 1; attempt <= handleAttempts; attempt++ {
		if err = c.handleTransaction(txHash, tx); err == nil {
			return nil
		}
		if attempt == handleAttempts || !isRetryable(err) {
			break
		}
		c.log.Warn("Solana retry handleTransaction", "hash", txHash, "attempt", attempt, "err", err)
		c.sleep(handleRetryDelay)
	}
	return err
}

func isRetryable(err error) bool {
	return err != nil && retryableErr.MatchString(err.Error())
}

func (c *Chain) alertFailure(program solana.PublicKey, txHash, stage string, err error) {
	c.state.RecordError("solana_"+stage, err.Error())
	c.log.Error("Solana tx filter failed, retry next round", "program", program, "hash", txHash, "stage", stage, "err", err)
	utils.Alarm(context.Background(), failureMessage(c.cfg.Id, program.String(), txHash, stage, err))
}

func failureMessage(chainId, program, txHash, stage string, err error) string {
	return strings.Join([]string{
		"Solana tx filter failed",
		"chainId=" + chainId,
		"address=" + program,
		"txHash=" + txHash,
		"stage=" + stage,
		fmt.Sprintf("retryable=%t", isRetryable(err)),
		"error=" + err.Error(),
	}, " ")
}

func cursorFile(program solana.PublicKey) string {
	return program.String() + ".cursor"
}

// loadCursors prefers the locally stored cursor over the configured startBlock, like compass-ts local_cursor.
func (c *Chain) loadCursors() {
	for _, program := range c.cfg.Mcs {
		cursor := c.cfg.StartBlock[program.String()]
		data, err := c.bs.ReadCustom(cursorFile(program))
		if err != nil {
			c.log.Error("Load solana cursor failed, fallback to startBlock", "program", program, "err", err)
		} else if s := strings.TrimSpace(string(data)); s != "" {
			cursor = s
		}
		c.cursors[program] = cursor
		c.log.Info("Solana sync start", "program", program, "cursor", cursor)
	}
}

func (c *Chain) saveCursor(program solana.PublicKey, cursor string) {
	if cursor == "" {
		return
	}
	if err := c.bs.CustomStore(cursorFile(program), []byte(cursor)); err != nil {
		c.log.Error("Save solana cursor failed", "program", program, "cursor", cursor, "err", err)
	}
}

// saveScanProgress publishes the lowest per-program progress: rows of every slot up to it are inserted.
func (c *Chain) saveScanProgress() {
	var scan uint64
	for i, program := range c.cfg.Mcs {
		p := c.progress[program]
		if p == 0 {
			return
		}
		if i == 0 || p < scan {
			scan = p
		}
	}
	if scan <= c.savedScan {
		return
	}
	c.savedScan = scan
	c.state.SetCurrentBlock(int64(scan))
	if c.isBackUp {
		return
	}
	for _, s := range c.storages {
		if err := s.ScanBlockNumber(c.cfg.Id, scan); err != nil {
			c.log.Error("Save scan height failed", "storage", s.Type(), "blockNumber", scan, "err", err)
		}
	}
}

// saveLatestBlock records the finalized slot; only the primary instance writes, unchanged values are skipped.
func (c *Chain) saveLatestBlock(latest uint64, now time.Time) {
	if c.isBackUp || latest == c.savedLatest || now.Sub(c.lastLatestSave) < latestSaveInterval {
		return
	}
	c.savedLatest = latest
	c.lastLatestSave = now
	for _, s := range c.storages {
		if err := s.LatestBlockNumber(c.cfg.Id, latest); err != nil {
			c.log.Error("Save latest height failed", "storage", s.Type(), "err", err)
		}
	}
}
