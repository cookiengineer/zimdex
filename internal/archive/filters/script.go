package filters

import (
	"regexp"
)

type ScriptFilter struct{}

func (f *ScriptFilter) Name() string        { return "strip_scripts" }
func (f *ScriptFilter) Description() string { return "Remove <script> tags and inline JavaScript from HTML" }

func (f *ScriptFilter) Detect(_ []byte, _ string) bool { return true }

func (f *ScriptFilter) FilterURL(rawURL string) string { return rawURL }

var scriptTagRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)

func (f *ScriptFilter) FilterHTML(htmlBody []byte, _ string) []byte {
	return scriptTagRegex.ReplaceAll(htmlBody, []byte{})
}
