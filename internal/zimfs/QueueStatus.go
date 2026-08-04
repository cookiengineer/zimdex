package zimfs

type QueueStatus string

const (
	QueueStatusPending     QueueStatus = "pending"
	QueueStatusDownloading QueueStatus = "downloading"
	QueueStatusDownloaded  QueueStatus = "downloaded"
	QueueStatusFailed      QueueStatus = "failed"
	QueueStatusSkipped     QueueStatus = "skipped"
)

