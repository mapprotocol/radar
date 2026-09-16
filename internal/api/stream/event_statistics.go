package stream

type EventStatisticsResp struct {
	Timezone  string                 `json:"timezone"`
	StartDate string                 `json:"start_date"`
	EndDate   string                 `json:"end_date"`
	UpdatedAt int64                  `json:"updated_at"`
	Days      []DailyEventStatistics `json:"days"`
}

type DailyEventStatistics struct {
	Date      string                 `json:"date"`
	UpdatedAt int64                  `json:"updated_at"`
	Chains    []ChainEventStatistics `json:"chains"`
}

type ChainEventStatistics struct {
	ChainId    string `json:"chain_id"`
	MessageOut int64  `json:"message_out"`
	MessageIn  int64  `json:"message_in"`
}
