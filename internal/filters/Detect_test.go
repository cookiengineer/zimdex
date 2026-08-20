package filters

import (
	"net/url"
	"testing"
)

func containsName(names []string, want string) bool {

	for _, name := range names {

		if name == want {
			return true
		}

	}

	return false

}

func TestDetectDefaultsOnly(t *testing.T) {

	names := Detect(nil, nil)

	if len(names) != 2 {
		t.Fatalf("Detect(nil, nil) = %v, want exactly 2 entries", names)
	}

	if containsName(names, "Trackers") == false || containsName(names, "Scripts") == false {
		t.Errorf("Detect(nil, nil) = %v, want Trackers and Scripts", names)
	}

}

func TestDetectPHPBB(t *testing.T) {

	names := Detect(nil, []byte(`<body id="phpbb" class="nojs notouch section-viewforum ltr">`))

	if containsName(names, "PHPBB") == false {
		t.Errorf("Detect(phpbb body) = %v, want to contain PHPBB", names)
	}

	if containsName(names, "MediaWiki") == true || containsName(names, "VBulletin") == true {
		t.Errorf("Detect(phpbb body) = %v, unexpected MediaWiki/VBulletin", names)
	}

}

func TestDetectMediaWiki(t *testing.T) {

	names := Detect(nil, []byte(`<meta name="generator" content="MediaWiki 1.39"/>`))

	if containsName(names, "MediaWiki") == false {
		t.Errorf("Detect(mediawiki meta) = %v, want to contain MediaWiki", names)
	}

}

func TestDetectVBulletin(t *testing.T) {

	names := Detect(nil, []byte(`<html><body id="vbulletin"`))

	if containsName(names, "VBulletin") == false {
		t.Errorf("Detect(vbulletin body) = %v, want to contain VBulletin", names)
	}

}

func TestDetectURLFallback(t *testing.T) {

	phpbb, _ := url.Parse("https://forum.example.com/viewforum.php?f=1")
	names := Detect(phpbb, nil)

	if containsName(names, "PHPBB") == false {
		t.Errorf("Detect(viewforum.php) = %v, want to contain PHPBB", names)
	}

}
