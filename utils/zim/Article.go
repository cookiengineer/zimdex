package zim

type Article struct {
	Type      uint16
	Title     string
	URLPtr    uint64
	Namespace byte
	url       string
	blob      uint32
	cluster   uint32
	zim       *ZimReader
}

func (article *Article) URL() string {
	return string(article.Namespace) + "/" + article.url
}

func (article *Article) Data() ([]byte, error) {

	if article.Type == TypeRedirect || article.Type == TypeLegacyLinkTarget || article.Type == TypeLegacyDeleted {
		return nil, nil
	}

	start, end, err0 := article.zim.clusterOffsetAtIdx(article.cluster)

	if err0 != nil {
		return nil, err0
	}

	tmp1, err1 := article.zim.bytesRangeAt(start, start+1)

	if err1 != nil {
		return nil, err1
	}

	compression := Compression(tmp1[0])

	if compression == CompressionLZMA {

		blobLookup := func() ([]byte, bool) {

			if value, ok := bcache.Get(article.cluster); ok {
				blob := value.([]byte)
				return blob, ok
			}

			return nil, false

		}

		blob := make([]byte, 0)
		ok := false

		if blob, ok = blobLookup(); !ok {

			tmp, err2 := article.zim.bytesRangeAt(start+1, end+1)

			if err2 == nil {

				buffer := bytes.NewBuffer(tmp)
				reader, err3 := NewXZReader(buffer)

				if err3 == nil {

					bytes, err4 := ioutil.ReadAll(reader)

					if err4 == nil {

						blob = make([]byte, len(bytes))
						copy(blob, bytes)

						// TODO: 2 requests for same blob could occur at the same time
						bcache.Add(article.cluster, blob)

					} else {
						return nil, err4
					}

					reader.Close()

				} else {
					return nil, err3
				}

			} else {
				return nil, err2
			}

		} else {

			tmp, ok := bcache.Get(article.cluster)

			if ok == true {
				blob = tmp.([]byte)
			} else {
				return nil, errors.New("Article cluster " + strconv.FormatUint(uint64(article.cluster), 10) + " not in cache anymore")
			}

		}

		if len(blob) >= (article.blob*4+8) {

			blob_start, err2 := readInt32(blob[article.blob*4:article.blob*4+4], nil)

			if err2 != nil {
				return nil, err2
			}

			blob_end, err3 := readInt32(blob[article.blob*4+4:article.blob*4+4+4], nil)

			if err3 != nil {
				return nil, err3
			}

			bytes := make([]byte, blob_end - blob_start)
			copy(bytes, blob[blob_start:blob_end])
			return bytes, nil

		}

	} else if compression == CompressionLegacyBZIP2 {

		return nil, errors.New("Unsupported ZIM legacy compression format bzip2 " + strconv.FormatUint(uint64(compression), 10))

	} else if compression == CompressionLegacyZLIB {

		return nil, errors.New("Unsupported ZIM legacy compression format zlib " + strconv.FormatUint(uint64(compression), 10))

	} else if compression == CompressionNone || compression == CompressionLegacyNone {

		blob_start, err2 := readInt32(article.zim.bytesRangeAt(uint64(article.blob*4+start+1), uint64(article.blob*4+start+1+4)))

		if err2 != nil {
			return nil, err2
		}

		blob_end, err3 := readInt32(article.zim.bytesRangeAt(uint64(article.blob*4+start+1+4), uint64(article.blob*4+start+1+4+4)))

		if err3 != nil {
			return nil, err3
		}

		return article.zim.bytesRangeAt(start+1+uint64(blob_start), start+1+uint64(blob_end))

	} else {

		return nil, errors.New("Unsupported ZIM compression format " + strconv.FormatUint(uint64(compression), 10))

	}

}

func (article *Article) MimeType() string {

	if article.Type == TypeRedirect || article.Type == TypeLegacyLinkTarget || article.Type == TypeLegacyDeleted {
		return ""
	}

	return article.zim.mimeTypeList[article.Type]

}
