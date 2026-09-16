package service

import (
	"context"
	"strconv"

	"github.com/mapprotocol/filter/internal/api/stream"
)

func (p *Event) Listening(ctx context.Context, chainID uint64) (*stream.ListeningEventsResp, error) {
	events, err := p.store.Listening(ctx, chainID)
	if err != nil {
		return nil, err
	}
	list := make([]*stream.ListeningEvent, 0, len(events))
	for _, event := range events {
		created := int64(0)
		if !event.CreatedAt.IsZero() {
			created = event.CreatedAt.Unix()
		}
		list = append(list, &stream.ListeningEvent{
			Id:          event.Id,
			ProjectId:   event.ProjectId,
			Address:     event.Address,
			Format:      event.Format,
			Topic:       event.Topic,
			BlockNumber: event.BlockNumber,
			Created:     created,
		})
	}
	return &stream.ListeningEventsResp{
		ChainId: strconv.FormatUint(chainID, 10),
		Total:   int64(len(list)),
		List:    list,
	}, nil
}
