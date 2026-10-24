package bridge

import (
	"fmt"
	"runtime"
	"strings"
)

// OpenURL opens an http or https URL in the default browser. Other schemes
// are refused, so a program on the server cannot launch arbitrary handlers.
func OpenURL(url string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("only http and https URLs are opened")
	}
	var err error
	if runtime.GOOS == "darwin" {
		_, err = run("open", url)
	} else {
		_, err = run("xdg-open", url)
	}
	if err != nil {
		return fmt.Errorf("could not open the browser: %w", err)
	}
	return nil
}
