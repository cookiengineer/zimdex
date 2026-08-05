package zimfs

type QueueInfo struct {
	Pending     int `json:"pending"`
	Downloading int `json:"downloading"`
	Downloaded  int `json:"downloaded"`
	Failed      int `json:"failed"`
	Skipped     int `json:"skipped"`
}
