package intranet

var onEventHandler = func(string, interface{}, []string) {}

func Init() {
	runServer()
}

// OnEvent sets intranet event handler
func OnEvent(fn func(string, interface{}, []string)) {
	onEventHandler = fn
}

func srEventHandler(uri string, subscribers []string, event interface{}) {
	if len(subscribers) == 0 {
		return
	}

	onEventHandler(uri, event, subscribers)
}
