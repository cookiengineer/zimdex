package urls

import net_url "net/url"
import "path/filepath"
import "strings"

var mime_types = map[string]string{
	".bmp":   "image/bmp",
	".bz2":   "application/x-bzip2",
	".css":   "text/css",
	".eot":   "application/vnd.ms-fontobject",
	".gif":   "image/gif",
	".gz":    "application/gzip",
	".html":  "text/html",
	".htm":   "text/html",
	".ico":   "image/x-icon",
	".jpeg":  "image/jpeg",
	".jpg":   "image/jpeg",
	".js":    "application/javascript",
	".json":  "application/json",
	".mjs":   "application/javascript",
	".mp3":   "audio/mpeg",
	".mp4":   "video/mp4",
	".ogg":   "audio/ogg",
	".pdf":   "application/pdf",
	".png":   "image/png",
	".svg":   "image/svg+xml",
	".tar":   "application/x-tar",
	".txt":   "text/plain",
	".ttf":   "font/ttf",
	".wav":   "audio/wav",
	".webm":  "video/webm",
	".webp":  "image/webp",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".xml":   "application/xml",
	".zip":   "application/zip",
}

func GetMimeType(url *net_url.URL) string {

	if url != nil {

		ext := strings.ToLower(filepath.Ext(url.Path))

		mime, ok := mime_types[ext]

		if ok == true {
			return mime
		} else {
			return "application/octet-stream"
		}

	} else {
		return "application/octet-stream"
	}

}
