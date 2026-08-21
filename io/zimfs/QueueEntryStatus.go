package zimfs

type QueueEntryStatus string

const (
	QueueEntryStatusPending     QueueEntryStatus = "pending"
	QueueEntryStatusDownloading QueueEntryStatus = "downloading"
	QueueEntryStatusDownloaded  QueueEntryStatus = "downloaded"
	QueueEntryStatusFailed      QueueEntryStatus = "failed"
	QueueEntryStatusSkipped     QueueEntryStatus = "skipped"
)

