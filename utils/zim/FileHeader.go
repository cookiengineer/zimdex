package zim

import "github.com/google/uuid"

type FileHeader struct {
	MajorVersion      uint16
	MinorVersion      uint16
	UUID              uuid.UUID
	EntryCount        uint32
	ClusterCount      uint32
	PathListPosition  uint64
	TitleListPosition uint64
	ClusterPosition   uint64
	MIMEListPosition  uint64
	MainPage          uint32
	LayoutPage        uint32
	CheckSumPosition  uint64
}

