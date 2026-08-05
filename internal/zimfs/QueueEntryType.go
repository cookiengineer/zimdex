package zimfs

type QueueEntryType string

const (
	QueueEntryTypePage         QueueEntryType = "page"
	QueueEntryTypeAsset        QueueEntryType = "asset"
	QueueEntryTypeExternalPage QueueEntryType = "external-page"
)
