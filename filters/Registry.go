package filters

var Registry = []Filter{
	&MediaWiki{},
	&PHPBB{},
	&VBulletin{},
	&Scripts{},
	&Trackers{},
}
