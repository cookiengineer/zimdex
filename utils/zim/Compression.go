package zim

type Compression uint8

// via https://wiki.openzim.org/wiki/ZIM_file_format
const (

	// Current Formats
	CompressionNone Compression = 1
	CompressionLZMA Compression = 4
	CompressionZSTD Compression = 5

	// Legacy Formats
	CompressionLegacyNone  Compression = 0
	CompressionLegacyZLIB  Compression = 2
	CompressionLegacyBZIP2 Compression = 3

)
