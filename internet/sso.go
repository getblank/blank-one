package internet

import (
	"fmt"
	"net/http"
	"strings"
)

var ssoHTML = `
    <!DOCTYPE html>
    <html lang="en">
        <head>
            <script type="text/javascript">
                var src, origin, allowed = [%s];
                function receiveMessage(event) {
                    if (allowed.indexOf(event.origin) < 0) { return; }
                    if (event.data === "remove") {
                        localStorage.removeItem("tempKey");
                        return;
                    }
                    src = event.source;
                    origin = event.origin;
                    src.postMessage(localStorage.getItem("tempKey"), origin);
                }
                window.addEventListener("message", receiveMessage, false);
                var k = localStorage.getItem("tempKey");
                window.setInterval(function () {
                    var _k = localStorage.getItem("tempKey");
                    if (_k !== k && src && origin) {
                        k = _k;
                        src.postMessage(localStorage.getItem("tempKey"), origin);
                    }
                }, 3000);
            </script>
        </head>
        <body></body>
    </html>
`

func ssoFrameHandler(w http.ResponseWriter, r *http.Request) {
	var ssoOrigins []string // TODO: extract from config directly
	// ssoOrigins := config.GetSSOOrigins()
	var origins string
	if len(ssoOrigins) > 0 {
		origins = `"` + strings.Join(ssoOrigins, `", "`) + `"`
	}

	res := fmt.Sprintf(ssoHTML, origins)
	htmlResponse(w, res)
}
