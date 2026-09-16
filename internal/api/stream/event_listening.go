package stream

type ListeningEvent struct {
	Id          int64  `json:"id"`
	ProjectId   int64  `json:"project_id"`
	Address     string `json:"address"`
	Format      string `json:"format"`
	Topic       string `json:"topic"`
	BlockNumber string `json:"block_number"`
	Created     int64  `json:"created"`
}

type ListeningEventsResp struct {
	ChainId string            `json:"chain_id"`
	Total   int64             `json:"total"`
	List    []*ListeningEvent `json:"list"`
}
