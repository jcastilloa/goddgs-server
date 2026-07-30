package chrome

import (
	"context"
	"os/exec"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestBrowserContextProtocolCompatibility(t *testing.T) {
	executable, err := exec.LookPath("google-chrome")
	if err != nil {
		t.Skip("google-chrome is not installed")
	}

	options := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(executable),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", "new"),
		chromedp.NoSandbox,
		chromedp.DisableGPU,
	}
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), options...)
	defer cancelAllocator()
	root, cancelRoot := chromedp.NewContext(allocator)
	defer cancelRoot()
	if err := chromedp.Run(root); err != nil {
		t.Fatal(err)
	}
	browser := &chromedpBrowser{context: root, cancelBrowser: cancelRoot, cancelAllocator: cancelAllocator}
	if browser == nil {
		t.Fatal("browser was not created")
	}
	defer browser.Close()

	pageContext, cancelPage := chromedp.NewContext(browser.context, chromedp.WithNewBrowserContext())
	defer cancelPage()
	if err := chromedp.Run(pageContext); err != nil {
		t.Fatalf("create browser-context target: %v", err)
	}
}
