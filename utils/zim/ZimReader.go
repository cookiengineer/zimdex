package zim

import "github.com/google/uuid"
import "bytes"
import "encoding/binary"
import "errors"
import "io"
import "strings"

type ZimReader struct {
	File          io.ReaderAt
	ArticleCount  uint32
	ClusterCount  uint32
	MimeTypeList  []string
	Header        *FileHeader

	urlPtrPos     uint64
	titlePtrPos   uint64
	clusterPtrPos uint64
	mime_list_position uint64
	mainPage      uint32
	layoutPage    uint32
}

func NewReader(file io.ReaderAt) (*ZimReader, error) {

	reader := ZimReader{
		File:         file,
		MimeTypeList: []string{},
		Header:       nil,
		MainPage:     0xffffffff,
		LayoutPage:   0xffffffff,
	}

	err0 := reader.readFileHeader()

	if err0 == nil {

		err1 := reader.readMIMETypes()

		if err1 == nil {
			return &reader, nil
		} else {
			return nil, err1
		}

	} else {
		return nil, err0
	}

}

func (reader *ZimReader) readFileHeader() error {

	magic, err0 := reader.readUint32(0)

	if err0 == nil && magic == 72173914 {

		version_major, err1 := reader.ReadUint16(4)
		version_minor, err2 := reader.ReadUint16(4+2)

		if err1 == nil && err2 == nil {

			if version_major == 5 {

				uuid_bytes, err00 := reader.ReadBytes(8, 8+16)
				entry_count, err3 := reader.ReadUint32(24)
				cluster_count, err4 := reader.ReadUint32(28)
				paths_pointer, err5 := reader.ReadUint64(32)
				titles_pointer, err6 := reader.ReadUint64(40)
				cluster_pointer, err7 := reader.ReadUint64(48)
				mimelist_pointer, err8 := reader.ReadUint64(56)
				mainpage, err9 := reader.ReadUint32(64)
				layoutpage, err10 := reader.ReadUint32(68)
				checksum_pointer, err11 := reader.ReadUint64(72)

				if err00 == nil && err3 == nil && err4 == nil && err5 == nil && err6 == nil && err7 == nil && err8 == nil && err9 == nil && err10 == nil && err11 == nil {

					var file_uuid uuid.UUID

					tmp01, err01 := uuid.FromBytes(uuid_bytes)

					if err01 == nil {
						file_uuid = tmp01
					} else {
						file_uuid = uuid.UUID(uuid_bytes)
					}

					reader.Header = &FileHeader{
						MajorVersion:      version_major,
						MinorVersion:      version_minor,
						UUID:              file_uuid,
						EntryCount:        entry_count,
						ClusterCount:      cluster_count,
						PathListPosition:  paths_pointer,
						TitleListPosition: titles_pointer,
						ClusterPosition:   cluster_pointer,
						MIMEListPosition:  mimelist_pointer,
						MainPage:          mainpage,
						LayoutPage:        layoutpage,
						ChecksumPosition:  checksum_pointer,
					}

					return nil

				} else if err3 != nil {
					return err3
				} else if err4 != nil {
					return err4
				} else if err5 != nil {
					return err5
				} else if err6 != nil {
					return err6
				} else if err7 != nil {
					return err7
				} else if err8 != nil {
					return err8
				} else if err9 != nil {
					return err9
				} else if err10 != nil {
					return err10
				} else if err11 != nil {
					return err11
				}

			} else if version_major == 6 {

				return errors.New("Unsupported ZIM file version " + strconv.FormatInt(int64(version), 10))

			} else {
				return errors.New("Unsupported ZIM file version " + strconv.FormatInt(int64(version), 10))
			}

		} else {
			return err2
		}

	} else {
		return errors.New("File is not a ZIM file")
	}

}

func (reader *ZimReader) readMIMETypes() error {

	if len(reader.MIMETypes) > 0 {

		// Already parsed
		return nil

	} else if reader.Header != nil {

		bytes, err0 := reader.ReadBytes(reader.Header.MIMEListPosition, reader.Header.MIMEListPosition + 4096)

		if err0 == nil {

			buffer := bytes.NewBuffer(bytes)
			filtered := make([]string, 0)

			for {

				line, err := buffer.ReadBytes('\x00')

				if err == nil {

					// Last line only contains the NULL byte
					if len(line) > 1 {
						filtered = append(filtered, strings.TrimRight(string(line), "\x00"))
					} else if len(line) == 1 {
						break
					}

				} else if err != nil {
					return err
				}

			}

			reader.MIMETypes = filtered

			return nil

		} else {
			return err0
		}

	} else {
		return errors.New("File is not a ZIM file")
	}

}

func (reader *ZimReader) ReadBytes(start uint64, end uint64) ([]byte, error) {

	bytes := make([]byte, end - start)
	amount, err0 := reader.file.ReadAt(bytes, int64(start))

	if err0 == nil {

		if amount == int(end - start) {
			return bytes, nil
		} else {
			return []byte{}, errors.New("EOF")
		}

	} else {
		return []byte{}, err0
	}

}

func (reader *ZimReader) ReadUint16(start uint64) (uint16, error) {

	bytes := make([]byte, 2)
	amount, err0 := reader.file.ReadAt(bytes, int64(start))

	if err0 == nil {

		if amount == 2 {
			return binary.LittleEndian.Uint16(bytes), nil
		} else {
			return 0, errors.New("EOF")
		}

	} else {
		return 0, err0
	}

}

func (reader *ZimReader) ReadUint32(start uint64) (uint32, error) {

	bytes := make([]byte, 4)
	amount, err0 := reader.file.ReadAt(bytes, int64(start))

	if err0 == nil {

		if amount == 4 {
			return binary.LittleEndian.Uint32(bytes), nil
		} else {
			return 0, errors.New("EOF")
		}

	} else {
		return 0, err0
	}

}

func (reader *ZimReader) ReadUint64(start uint64) (uint64, error) {

	bytes := make([]byte, 8)
	amount, err0 := reader.file.ReadAt(bytes, int64(start))

	if err0 == nil {

		if amount == 8 {
			return binary.LittleEndian.Uint64(bytes), nil
		} else {
			return 0, errors.New("EOF")
		}

	} else {
		return 0, err0
	}

}

func (reader *ZimReader) ReadEntries() <-chan *Entry {

	entry_count := uint32(0)

	if reader.Header != nil {
		entry_count = reader.Header.EntryCount
	}

	channel := make(chan *Entry, entry_count)

	go func() {

		for id := uint32(1); id < entry_count; id++ {

			entry, err := reader.ReadEntry(uint32(id))

			if err == nil {

				if entry.Type == TypeRedirect {

					// TODO: Handle Redirect Entries
					channel <- nil

				} else {
					channel <- entry
				}

			} else {
				channel <- nil
			}

		}

		close(channel)

	}()

	return channel

}

func (reader *ZimReader) ReadEntry(id uint32) (*Entry, error) {

	if reader.Header != nil {

		offset, err0 := reader.ReadUint64(reader.Header.PathListPosition + uint64(id) * 8)

		if err0 == nil {

			// TODO: Entry starts at ReadBytes(offset, ...)
			// TODO: Port old FillArticleAt() method

		} else {
			return err0
		}

	} else {
		return errors.New("File is not a ZIM file")
	}

}

