package trigger

import (
	"context"
	"log/slog"

	"github.com/meshcore-go/OwlShack/internal/config"
	"github.com/mmcdole/gofeed"
)

// RSSTrigger fires once per new item in an RSS, Atom or JSON feed, from the feed entry alone.
type RSSTrigger struct{ *feedPoller }

func NewRSSTrigger(botName string, cfg config.TriggerConfig, log *slog.Logger) (*RSSTrigger, error) {
	p, err := newFeedPoller("rss", botName, cfg, log)
	if err != nil {
		return nil, err
	}
	p.decode = decodeEntry
	return &RSSTrigger{p}, nil
}

// decodeEntry is what every feed carries, with no second fetch.
func decodeEntry(_ context.Context, feed *gofeed.Feed, item *gofeed.Item) (map[string]any, string, error) {
	data := map[string]any{
		"Feed":        feed.Title,
		"FeedLink":    feed.Link,
		"ID":          itemID(item),
		"Title":       item.Title,
		"Link":        item.Link,
		"Description": item.Description,
		"Content":     item.Content,
		"Author":      authorName(item),
		"Categories":  item.Categories,
		"Published":   itemTime(item),
	}
	return data, item.Title + "\n" + item.Description, nil
}

var _ Trigger = (*RSSTrigger)(nil)
