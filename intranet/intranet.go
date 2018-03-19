package intranet

var onEventHandler = func(string, interface{}, []string) {}

func Init() {
	runServer()
}

// OnEvent sets intranet event handler
func OnEvent(fn func(string, interface{}, []string)) {
	onEventHandler = fn
}
