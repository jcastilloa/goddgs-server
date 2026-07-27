package goddgs

import (
	"context"
	"errors"
	"net"
	"net/url"
	"syscall"
	"time"

	ddgs "github.com/jcastilloa/goddgs"
	"github.com/jcastilloa/goddgs-server/search/domain"
	extractAIApplication "github.com/jcastilloa/goddgs-server/shared/extractai/application"
)

type ddgsClient struct {
	source sourceClient
}

type sourceClient interface {
	Text(context.Context, string, ...ddgs.SearchOption) ([]ddgs.RawResult, error)
	Images(context.Context, string, ...ddgs.SearchOption) ([]ddgs.RawResult, error)
	News(context.Context, string, ...ddgs.SearchOption) ([]ddgs.RawResult, error)
	Videos(context.Context, string, ...ddgs.SearchOption) ([]ddgs.RawResult, error)
	Books(context.Context, string, ...ddgs.SearchOption) ([]ddgs.RawResult, error)
	Extract(context.Context, string, ...ddgs.ExtractOption) (ddgs.ExtractResult, error)
}

func NewClient(proxy string, timeout time.Duration) Client {
	return newClient(proxy, timeout, func(options ...ddgs.Option) sourceClient {
		return ddgs.New(options...)
	})
}

func newClient(proxy string, timeout time.Duration, create func(...ddgs.Option) sourceClient) Client {
	options := []ddgs.Option{ddgs.WithTimeout(timeout)}
	if proxy != "" {
		options = append(options, ddgs.WithProxy(proxy))
	}
	return ddgsClient{source: create(options...)}
}

func (c ddgsClient) Search(ctx context.Context, request domain.SearchRequest) ([]domain.RawResult, error) {
	results, err := c.search(ctx, request)
	if err != nil {
		return nil, classifySourceError(err)
	}
	return convertResults(results), nil
}

func (c ddgsClient) Extract(ctx context.Context, request domain.ExtractRequest) (domain.ExtractResult, error) {
	options := extractOptions(request)
	result, err := c.source.Extract(ctx, request.URL, options...)
	if err != nil {
		return domain.ExtractResult{}, classifySourceError(err)
	}
	if request.Format == "html" {
		content, err := extractAIApplication.RenderMarkdownHTML(extractedText(result.Content))
		if err != nil {
			return domain.ExtractResult{}, err
		}
		return domain.ExtractResult{URL: result.URL, Content: content}, nil
	}
	return domain.ExtractResult{URL: result.URL, Content: result.Content}, nil
}

func (c ddgsClient) search(ctx context.Context, request domain.SearchRequest) ([]ddgs.RawResult, error) {
	options := searchOptions(request)
	switch request.Category {
	case domain.CategoryText:
		return c.source.Text(ctx, request.Query, options...)
	case domain.CategoryImages:
		return c.source.Images(ctx, request.Query, options...)
	case domain.CategoryNews:
		return c.source.News(ctx, request.Query, options...)
	case domain.CategoryVideos:
		return c.source.Videos(ctx, request.Query, options...)
	case domain.CategoryBooks:
		return c.source.Books(ctx, request.Query, options...)
	default:
		return nil, domain.ErrInvalidSearchRequest
	}
}

func searchOptions(request domain.SearchRequest) []ddgs.SearchOption {
	options := make([]ddgs.SearchOption, 0, 12)
	if request.Region != "" {
		options = append(options, ddgs.WithRegion(request.Region))
	}
	if request.SafeSearch != "" {
		options = append(options, ddgs.WithSafeSearch(request.SafeSearch))
	}
	if request.TimeLimit != "" {
		options = append(options, ddgs.WithTimeLimit(request.TimeLimit))
	}
	if request.MaxResults != nil {
		options = append(options, ddgs.WithMaxResults(*request.MaxResults))
	}
	if request.Page != nil {
		options = append(options, ddgs.WithPage(*request.Page))
	}
	if request.Backend != "" {
		options = append(options, ddgs.WithBackend(request.Backend))
	}
	if request.Category == domain.CategoryImages {
		options = appendImageOptions(options, request.Images)
	}
	if request.Category == domain.CategoryVideos {
		options = appendVideoOptions(options, request.Videos)
	}
	if request.Diagnostics != nil && request.Diagnostics.OnComplete != nil {
		options = append(options, ddgs.WithSearchDiagnostics(func(diagnostic ddgs.SearchDiagnostic) {
			request.Diagnostics.OnComplete(domain.SearchDiagnostic{
				Backend:     diagnostic.Backend,
				Provider:    diagnostic.Provider,
				ResultCount: diagnostic.ResultCount,
				Err:         diagnostic.Err,
			})
		}))
	}
	return options
}

func appendImageOptions(options []ddgs.SearchOption, image domain.ImageOptions) []ddgs.SearchOption {
	if image.Size != "" {
		options = append(options, ddgs.WithImageSize(image.Size))
	}
	if image.Color != "" {
		options = append(options, ddgs.WithImageColor(image.Color))
	}
	if image.Type != "" {
		options = append(options, ddgs.WithImageType(image.Type))
	}
	if image.Layout != "" {
		options = append(options, ddgs.WithImageLayout(image.Layout))
	}
	if image.License != "" {
		options = append(options, ddgs.WithImageLicense(image.License))
	}
	return options
}

func appendVideoOptions(options []ddgs.SearchOption, video domain.VideoOptions) []ddgs.SearchOption {
	if video.Resolution != "" {
		options = append(options, ddgs.WithVideoResolution(video.Resolution))
	}
	if video.Duration != "" {
		options = append(options, ddgs.WithVideoDuration(video.Duration))
	}
	if video.License != "" {
		options = append(options, ddgs.WithVideoLicense(video.License))
	}
	return options
}

func extractOptions(request domain.ExtractRequest) []ddgs.ExtractOption {
	if request.Format == "" {
		return nil
	}
	if request.Format == "html" {
		return []ddgs.ExtractOption{ddgs.WithExtractFormat("text_markdown")}
	}
	return []ddgs.ExtractOption{ddgs.WithExtractFormat(request.Format)}
}

func extractedText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return ""
	}
}

func convertResults(results []ddgs.RawResult) []domain.RawResult {
	converted := make([]domain.RawResult, len(results))
	for index, result := range results {
		converted[index] = domain.RawResult(result)
	}
	return converted
}

func classifySourceError(err error) error {
	if errors.Is(err, ddgs.ErrRateLimit) {
		return errors.Join(domain.ErrRateLimited, err)
	}
	if errors.Is(err, ddgs.ErrTimeout) {
		return errors.Join(domain.ErrSearchTimeout, err)
	}
	if !isTransportError(err) {
		return err
	}
	return errors.Join(ErrTransport, err)
}

func isTransportError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return true
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) {
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, syscall.EPIPE)
}
