package filters

var Registry = []Filter{
	&TrackingParameters{},
	&MediaWiki{},
	&Scripts{},
}
