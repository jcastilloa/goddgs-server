package chrome

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"
)

func NewChromedpFactory(executablePath string) BrowserFactory {
	return chromedpFactory(executablePath, launchChromedp)
}

func NewChromedpFactoryWithLocator(locator *ExecutableLocator) BrowserFactory {
	if locator == nil {
		return NewChromedpFactory("")
	}
	return chromedpFactoryWithPath(locator.ExecutablePath, launchChromedp)
}

type chromeLauncher func(context.Context, string, string) (Browser, error)

func chromedpFactory(executablePath string, launch chromeLauncher) BrowserFactory {
	return chromedpFactoryWithPath(func() string { return executablePath }, launch)
}

func chromedpFactoryWithPath(executablePath func() string, launch chromeLauncher) BrowserFactory {
	return func(ctx context.Context, proxyURL string) (Browser, error) {
		executable := ""
		if executablePath != nil {
			executable = strings.TrimSpace(executablePath())
		}
		if executable == "" {
			var found bool
			executable, found = findChromeExecutable()
			if !found {
				return nil, ErrUnavailable
			}
		}
		browser, err := launch(ctx, executable, strings.TrimSpace(proxyURL))
		if err != nil || browser == nil {
			return nil, ErrUnavailable
		}
		return browser, nil
	}
}

func launchChromedp(ctx context.Context, executable, proxyURL string) (Browser, error) {
	options := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	options = append(options, chromedp.ExecPath(executable))
	if proxyURL != "" {
		options = append(options, chromedp.ProxyServer(proxyURL))
	}
	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(ctx, options...)
	rootCtx, cancelRoot := chromedp.NewContext(allocatorCtx)
	if err := chromedp.Run(rootCtx); err != nil {
		cancelRoot()
		cancelAllocator()
		return nil, err
	}
	return &chromedpBrowser{context: rootCtx, cancelBrowser: cancelRoot, cancelAllocator: cancelAllocator}, nil
}

type chromedpBrowser struct {
	context         context.Context
	cancelBrowser   context.CancelFunc
	cancelAllocator context.CancelFunc
	once            sync.Once
	err             error
}

func (b *chromedpBrowser) Close() error {
	b.once.Do(func() {
		b.err = chromedp.Cancel(b.context)
		if errors.Is(b.err, context.Canceled) {
			b.err = nil
		}
		b.cancelBrowser()
		b.cancelAllocator()
	})
	return b.err
}

type chromedpPageRunner struct{}

func (chromedpPageRunner) Run(ctx context.Context, browser Browser, _ string, rawURL string) (PageResult, error) {
	chromeBrowser, ok := browser.(*chromedpBrowser)
	if !ok || chromeBrowser == nil {
		return PageResult{}, ErrUnavailable
	}
	pageCtx, cancelPage := chromedp.NewContext(chromeBrowser.context, chromedp.WithNewBrowserContext())
	defer cancelPage()
	stop := context.AfterFunc(ctx, cancelPage)
	defer stop()

	var html, finalURL string
	err := chromedp.Run(pageCtx,
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
		chromedp.Location(&finalURL),
	)
	if err != nil {
		if ctx.Err() != nil {
			return PageResult{}, ctx.Err()
		}
		if errors.Is(context.Cause(pageCtx), ErrUnavailable) {
			return PageResult{}, ErrUnavailable
		}
		return PageResult{}, err
	}
	return PageResult{URL: finalURL, HTML: html}, nil
}
