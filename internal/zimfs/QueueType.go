package zimfs

type QueueType string

const (
	QueueTypePage         QueueType = "page"
	QueueTypeAsset        QueueType = "asset"
	QueueTypeExternalPage QueueType = "external-page"
)
