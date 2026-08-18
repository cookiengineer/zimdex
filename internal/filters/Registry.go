package filters

var Registry = []Filter{
	&MediaWiki{},
	&Scripts{},
	&Trackers{},
}
