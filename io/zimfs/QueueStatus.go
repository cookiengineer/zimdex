package zimfs

type QueueStatus string

const (
	QueueStatusIdle    QueueStatus = "idle"
	QueueStatusRunning QueueStatus = "running"
	QueueStatusPaused  QueueStatus = "paused"
)
