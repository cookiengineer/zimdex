package zim

type Type uint16

// via https://wiki.openzim.org/wiki/ZIM_file_format
const (

	// Current Types
	TypeRedirect         uint16 = 0xffff

	// Legacy Types
	TypeLegacyLinkTarget uint16 = 0xfffe
	TypeLegacyDeleted    uint16 = 0xfffd

)
