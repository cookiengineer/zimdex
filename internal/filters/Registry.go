package filters

var Registry = []Filter{
	&MediaWiki{},
	&PHPBB{},
	&Scripts{},
	&Trackers{},
}
