package zim

import "bytes"
import "errors"
import "fmt"
import "io/ioutil"
import "strings"
import "sync"

const RedirectEntry   uint16 = 0xffff
const LinkTargetEntry uint16 = 0xfffe
const DeletedEntry    uint16 = 0xfffd

// convenient method to return the Article at URL index idx
func (z *ZimReader) ArticleAtURLIdx(idx uint32) (*Article, error) {
	o, err := z.OffsetAtURLIdx(idx)
	if err != nil {
		return nil, err
	}
	return z.ArticleAt(o)
}

// return the article main page if it exists
func (z *ZimReader) MainPage() (*Article, error) {
	if z.mainPage == 0xffffffff {
		return nil, nil
	}
	return z.ArticleAtURLIdx(z.mainPage)
}

// get the article (Directory) pointed by the offset found in URLpos or Titlepos
func (z *ZimReader) ArticleAt(offset uint64) (*Article, error) {
	a := articlePool.Get().(*Article)
	err := z.FillArticleAt(a, offset)
	return a, err
}

// Fill an article with datas found at offset
func (z *ZimReader) FillArticleAt(a *Article, offset uint64) error {
	a.z = z
	a.URLPtr = offset

	mimeIdx, err := readInt16(z.bytesRangeAt(offset, offset+2))
	a.EntryType = mimeIdx

	// Linktarget or Target Entry
	if mimeIdx == LinkTargetEntry || mimeIdx == DeletedEntry {
		//TODO
		return nil
	}

	s, err := z.bytesRangeAt(offset+3, offset+4)
	if err != nil {
		return err
	}
	a.Namespace = s[0]

	a.cluster, err = readInt32(z.bytesRangeAt(offset+8, offset+8+4))
	if err != nil {
		return err
	}
	a.blob, err = readInt32(z.bytesRangeAt(offset+12, offset+12+4))
	if err != nil {
		return err
	}

	// Redirect
	if mimeIdx == RedirectEntry {
		// assume the url + title won't be longer than 2k
		b, err := z.bytesRangeAt(offset+12, offset+12+2048)
		if err != nil {
			return nil
		}
		bbuf := bytes.NewBuffer(b)
		a.url, err = bbuf.ReadString('\x00')
		if err != nil {
			return err
		}
		a.url = strings.TrimRight(string(a.url), "\x00")

		a.Title, err = bbuf.ReadString('\x00')
		if err != nil {
			return err
		}
		a.Title = strings.TrimRight(string(a.Title), "\x00")
		return err
	}

	b, err := z.bytesRangeAt(offset+16, offset+16+2048)
	if err != nil {
		return nil
	}
	bbuf := bytes.NewBuffer(b)
	a.url, err = bbuf.ReadString('\x00')
	if err != nil {
		return err
	}

	a.url = strings.TrimRight(string(a.url), "\x00")

	title, err := bbuf.ReadString('\x00')
	if err != nil {
		return err
	}
	title = strings.TrimRight(string(title), "\x00")
	// This is a trick to force a copy and avoid retain of the full buffer
	// mainly for indexing title reasons
	if len(title) != 0 {
		a.Title = title[0:1] + title[1:]
	}
	return nil
}

func (a *Article) String() string {
	return fmt.Sprintf("Mime: 0x%x URL: [%s], Title: [%s], Cluster: 0x%x Blob: 0x%x",
		a.EntryType, a.FullURL(), a.Title, a.cluster, a.blob)
}

// RedirectIndex return the redirect index of RedirectEntry type article
// return an err if not a redirect entry
func (a *Article) RedirectIndex() (uint32, error) {
	if a.EntryType != RedirectEntry {
		return 0, errors.New("Not a RedirectEntry")
	}
	// We use the cluster to save the redirect index position for RedirectEntry type
	return a.cluster, nil
}

func (a *Article) blobOffsetsAtIdx(z *ZimReader) (start, end uint64) {
	idx := a.blob
	offset := z.clusterPtrPos + uint64(idx)*8
	start, err := readInt64(z.bytesRangeAt(offset, offset+8))
	if err != nil {
		return
	}
	offset = z.clusterPtrPos + uint64(idx+1)*8
	end, err = readInt64(z.bytesRangeAt(offset, offset+8))

	return
}
